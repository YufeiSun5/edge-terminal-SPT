package reports

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/afero"
	"github.com/xuri/excelize/v2"
)

const reportXLSXContentType = "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
const pngContentType = "image/png"

func TestReportArtifactStoreExcelizeSpike(t *testing.T) {
	store := newLocalSpikeStore(t, t.TempDir())
	historyRows := sampleHistoryRows()
	avgValue := averageHistoryValue(historyRows)
	chartBytes := buildHistoryTrendPNG(t, historyRows, 10, 20)
	chartKey := "reports/edge-local/AC-01/2026/237_AC01-LONG/requests/1/generations/1/charts/supply-air-temperature.png"
	if err := store.Put(context.Background(), chartKey, chartBytes, pngContentType); err != nil {
		t.Fatalf("write generated chart artifact: %v", err)
	}
	chartFromStore, chartMeta, err := store.Get(context.Background(), chartKey)
	if err != nil {
		t.Fatalf("read generated chart artifact: %v", err)
	}
	if chartMeta.ContentType != pngContentType || chartMeta.SHA256 != sha256Hex(chartBytes) {
		t.Fatalf("chart metadata = %+v", chartMeta)
	}
	if _, err := png.Decode(bytes.NewReader(chartFromStore)); err != nil {
		t.Fatalf("generated chart is not a valid png: %v", err)
	}

	templateBytes := buildReportTemplateWorkbook(t)
	templateKey := "templates/ac-performance/v1/template-hash/source.xlsx"
	if err := store.Put(context.Background(), templateKey, templateBytes, reportXLSXContentType); err != nil {
		t.Fatalf("write template artifact: %v", err)
	}

	templateFromStore, meta, err := store.Get(context.Background(), templateKey)
	if err != nil {
		t.Fatalf("read template artifact: %v", err)
	}
	if meta.ContentType != reportXLSXContentType || meta.SHA256 != sha256Hex(templateBytes) {
		t.Fatalf("template metadata = %+v", meta)
	}
	report, err := excelize.OpenReader(bytes.NewReader(templateFromStore))
	if err != nil {
		t.Fatalf("open template from artifact store: %v", err)
	}
	defer func() { _ = report.Close() }()

	const sheet = "Report"
	if err := report.SetCellValue(sheet, "B2", "AC-01"); err != nil {
		t.Fatalf("write mapped task field: %v", err)
	}
	if err := report.SetCellValue(sheet, "B3", avgValue); err != nil {
		t.Fatalf("write mapped metric field: %v", err)
	}
	if err := report.SetCellValue(sheet, "D3", 20); err != nil {
		t.Fatalf("write upper limit field: %v", err)
	}
	if err := report.SetCellFormula(sheet, "B4", "=B3*2"); err != nil {
		t.Fatalf("write formula field: %v", err)
	}
	if err := report.AddPictureFromBytes(sheet, "F2", &excelize.Picture{
		Extension: ".png",
		File:      chartFromStore,
		Format: &excelize.GraphicOptions{
			AltText: "temperature trend",
		},
	}); err != nil {
		t.Fatalf("insert chart png: %v", err)
	}

	reportBuffer, err := report.WriteToBuffer()
	if err != nil {
		t.Fatalf("serialize generated report: %v", err)
	}
	reportBytes := reportBuffer.Bytes()
	reportKey := "reports/edge-local/AC-01/2026/237_AC01-LONG/requests/1/generations/1/report.xlsx"
	if err := store.Put(context.Background(), reportKey, reportBytes, reportXLSXContentType); err != nil {
		t.Fatalf("write generated report artifact: %v", err)
	}

	generatedFromStore, meta, err := store.Get(context.Background(), reportKey)
	if err != nil {
		t.Fatalf("read generated report artifact: %v", err)
	}
	if meta.ContentType != reportXLSXContentType || meta.SHA256 != sha256Hex(reportBytes) {
		t.Fatalf("generated metadata = %+v", meta)
	}
	generated, err := excelize.OpenReader(bytes.NewReader(generatedFromStore))
	if err != nil {
		t.Fatalf("open generated report from artifact store: %v", err)
	}
	defer func() { _ = generated.Close() }()

	assertCellValue(t, generated, sheet, "B2", "AC-01")
	assertCellValue(t, generated, sheet, "B3", "14.25")
	assertCellValue(t, generated, sheet, "D3", "20")
	formula, err := generated.GetCellFormula(sheet, "B4")
	if err != nil {
		t.Fatalf("read formula: %v", err)
	}
	if !strings.Contains(formula, "B3*2") {
		t.Fatalf("formula not preserved: %q", formula)
	}
	calculated, err := generated.CalcCellValue(sheet, "B4")
	if err != nil {
		t.Fatalf("calculate formula: %v", err)
	}
	if calculated != "28.5" {
		t.Fatalf("calculated formula value = %q, want 28.5", calculated)
	}
	pictures, err := generated.GetPictures(sheet, "F2")
	if err != nil {
		t.Fatalf("read inserted picture: %v", err)
	}
	if len(pictures) != 1 {
		t.Fatalf("picture count = %d, want 1", len(pictures))
	}
	if pictures[0].Extension != ".png" || len(pictures[0].File) == 0 {
		t.Fatalf("unexpected picture metadata: extension=%q bytes=%d", pictures[0].Extension, len(pictures[0].File))
	}
}

type historySample struct {
	CollectedAt time.Time
	Value       float64
	Quality     string
}

type localSpikeStore struct {
	fs   afero.Fs
	root string
	meta map[string]artifactMeta
}

type artifactMeta struct {
	ContentType string
	SHA256      string
	Size        int64
}

func newLocalSpikeStore(t *testing.T, root string) *localSpikeStore {
	t.Helper()
	return &localSpikeStore{fs: afero.NewOsFs(), root: root, meta: map[string]artifactMeta{}}
}

func (s *localSpikeStore) Put(_ context.Context, key string, raw []byte, contentType string) error {
	path, err := s.safePath(key)
	if err != nil {
		return err
	}
	if err := s.fs.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if err := afero.WriteFile(s.fs, path, raw, 0o644); err != nil {
		return err
	}
	s.meta[key] = artifactMeta{ContentType: contentType, SHA256: sha256Hex(raw), Size: int64(len(raw))}
	return nil
}

func (s *localSpikeStore) Get(_ context.Context, key string) ([]byte, artifactMeta, error) {
	path, err := s.safePath(key)
	if err != nil {
		return nil, artifactMeta{}, err
	}
	raw, err := afero.ReadFile(s.fs, path)
	if err != nil {
		return nil, artifactMeta{}, err
	}
	meta := s.meta[key]
	return raw, meta, nil
}

func (s *localSpikeStore) safePath(key string) (string, error) {
	if strings.TrimSpace(key) == "" || filepath.IsAbs(key) || strings.Contains(key, "..") {
		return "", fs.ErrInvalid
	}
	clean := filepath.Clean(filepath.FromSlash(key))
	if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fs.ErrInvalid
	}
	return filepath.Join(s.root, clean), nil
}

func buildReportTemplateWorkbook(t *testing.T) []byte {
	t.Helper()
	file := excelize.NewFile()
	defer func() { _ = file.Close() }()

	const sheet = "Report"
	index, err := file.NewSheet(sheet)
	if err != nil {
		t.Fatalf("create report sheet: %v", err)
	}
	file.SetActiveSheet(index)
	if err := file.DeleteSheet("Sheet1"); err != nil {
		t.Fatalf("delete default sheet: %v", err)
	}
	rows := [][]any{
		{"Customer Template"},
		{"Device", ""},
		{"Average", ""},
		{"Formula", ""},
		{"Upper Limit", ""},
	}
	for rowIndex, row := range rows {
		cell, err := excelize.CoordinatesToCellName(1, rowIndex+1)
		if err != nil {
			t.Fatalf("cell name: %v", err)
		}
		if err := file.SetSheetRow(sheet, cell, &row); err != nil {
			t.Fatalf("write template row %d: %v", rowIndex+1, err)
		}
	}
	if err := file.SetCellFormula(sheet, "B4", "=B3*2"); err != nil {
		t.Fatalf("write template formula: %v", err)
	}
	buffer, err := file.WriteToBuffer()
	if err != nil {
		t.Fatalf("serialize template: %v", err)
	}
	return buffer.Bytes()
}

func sampleHistoryRows() []historySample {
	base := time.Date(2026, 6, 16, 9, 20, 0, 0, time.Local)
	return []historySample{
		{CollectedAt: base, Value: 11.2, Quality: "good"},
		{CollectedAt: base.Add(10 * time.Minute), Value: 12.4, Quality: "good"},
		{CollectedAt: base.Add(20 * time.Minute), Value: 12.1, Quality: "good"},
		{CollectedAt: base.Add(30 * time.Minute), Value: 14.8, Quality: "good"},
		{CollectedAt: base.Add(40 * time.Minute), Value: 15.7, Quality: "good"},
		{CollectedAt: base.Add(50 * time.Minute), Value: 16.3, Quality: "good"},
		{CollectedAt: base.Add(60 * time.Minute), Value: 17.25, Quality: "good"},
	}
}

func averageHistoryValue(rows []historySample) float64 {
	var sum float64
	var count float64
	for _, row := range rows {
		if row.Quality != "good" {
			continue
		}
		sum += row.Value
		count++
	}
	if count == 0 {
		return 0
	}
	return sum / count
}

func buildHistoryTrendPNG(t *testing.T, rows []historySample, lowerLimit float64, upperLimit float64) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 220, 96))
	fill(img, color.RGBA{R: 255, G: 255, B: 255, A: 255})
	axis := color.RGBA{R: 90, G: 96, B: 105, A: 255}
	line := color.RGBA{R: 37, G: 99, B: 235, A: 255}
	limit := color.RGBA{R: 220, G: 38, B: 38, A: 255}
	drawLine(img, 18, 78, 206, 78, axis)
	drawLine(img, 18, 14, 18, 78, axis)
	minValue := lowerLimit
	maxValue := upperLimit
	for _, row := range rows {
		if row.Value < minValue {
			minValue = row.Value
		}
		if row.Value > maxValue {
			maxValue = row.Value
		}
	}
	if maxValue <= minValue {
		maxValue = minValue + 1
	}
	drawLine(img, 18, yForValue(upperLimit, minValue, maxValue), 206, yForValue(upperLimit, minValue, maxValue), limit)
	drawLine(img, 18, yForValue(lowerLimit, minValue, maxValue), 206, yForValue(lowerLimit, minValue, maxValue), limit)
	points := make([][2]int, 0, len(rows))
	for i, row := range rows {
		x := 18
		if len(rows) > 1 {
			x = 18 + int(float64(188*i)/float64(len(rows)-1))
		}
		points = append(points, [2]int{x, yForValue(row.Value, minValue, maxValue)})
	}
	for i := 1; i < len(points); i++ {
		drawLine(img, points[i-1][0], points[i-1][1], points[i][0], points[i][1], line)
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode trend png: %v", err)
	}
	return buf.Bytes()
}

func yForValue(value float64, minValue float64, maxValue float64) int {
	ratio := (value - minValue) / (maxValue - minValue)
	return 78 - int(ratio*64)
}

func fill(img *image.RGBA, c color.RGBA) {
	for y := img.Bounds().Min.Y; y < img.Bounds().Max.Y; y++ {
		for x := img.Bounds().Min.X; x < img.Bounds().Max.X; x++ {
			img.SetRGBA(x, y, c)
		}
	}
}

func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA) {
	dx := abs(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -abs(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		img.SetRGBA(x0, y0, c)
		if x0 == x1 && y0 == y1 {
			return
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func sha256Hex(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
