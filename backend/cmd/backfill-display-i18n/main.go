package main

import (
	"flag"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"spindle-edge/backend/internal/config"
	"spindle-edge/backend/internal/database"
)

type projectRow struct {
	ID            uint
	ProjectCode   string
	Name          string
	DisplayName   string
	DisplayNameEN string
	DisplayNameJA string
}

type tagRow struct {
	VarID         int64
	VarName       string
	DisplayName   string
	DisplayNameEN string
	DisplayNameJA string
}

type translation struct {
	EN string
	JA string
}

var zhTranslations = map[string]translation{
	"1号精密空调":    {EN: "Precision AC 1", JA: "精密空調1号機"},
	"2号精密空调":    {EN: "Precision AC 2", JA: "精密空調2号機"},
	"3号精密空调":    {EN: "Precision AC 3", JA: "精密空調3号機"},
	"4号精密空调":    {EN: "Precision AC 4", JA: "精密空調4号機"},
	"5号精密空调":    {EN: "Precision AC 5", JA: "精密空調5号機"},
	"6号精密空调":    {EN: "Precision AC 6", JA: "精密空調6号機"},
	"送风温度":      {EN: "Supply Air Temperature", JA: "給気温度"},
	"送风湿度":      {EN: "Supply Air Humidity", JA: "給気湿度"},
	"运行状态":      {EN: "Run Status", JA: "運転状態"},
	"吹出口湿度":     {EN: "Outlet Humidity", JA: "吹出口湿度"},
	"吹出口温度":     {EN: "Outlet Temperature", JA: "吹出口温度"},
	"风速1":       {EN: "Air Speed 1", JA: "風速1"},
	"风速2":       {EN: "Air Speed 2", JA: "風速2"},
	"风速3":       {EN: "Air Speed 3", JA: "風速3"},
	"风速4":       {EN: "Air Speed 4", JA: "風速4"},
	"风速5":       {EN: "Air Speed 5", JA: "風速5"},
	"回风温度":      {EN: "Return Air Temperature", JA: "還気温度"},
	"回风湿度":      {EN: "Return Air Humidity", JA: "還気湿度"},
	"进风温度":      {EN: "Inlet Air Temperature", JA: "入口空気温度"},
	"进风湿度":      {EN: "Inlet Air Humidity", JA: "入口空気湿度"},
	"吸入口温度":     {EN: "Inlet Temperature", JA: "吸入口温度"},
	"吸入口湿度":     {EN: "Inlet Humidity", JA: "吸入口湿度"},
	"吸入风量":      {EN: "Intake Air Volume", JA: "吸入風量"},
	"压缩机吸气温度":   {EN: "Compressor Suction Temperature", JA: "圧縮機吸入温度"},
	"压缩机排气温度":   {EN: "Compressor Discharge Temperature", JA: "圧縮機吐出温度"},
	"压缩机吐出口温度":  {EN: "Compressor Discharge Temperature", JA: "圧縮機吐出口温度"},
	"压缩机吸入管温度":  {EN: "Compressor Suction Pipe Temperature", JA: "圧縮機吸入管温度"},
	"蒸发器出口温度":   {EN: "Evaporator Outlet Temperature", JA: "蒸発器出口温度"},
	"冷凝器出口温度":   {EN: "Condenser Outlet Temperature", JA: "凝縮器出口温度"},
	"膨胀阀出口温度":   {EN: "Expansion Valve Outlet Temperature", JA: "膨張弁出口温度"},
	"冷却水进口温度":   {EN: "Cooling Water Inlet Temperature", JA: "冷却水入口温度"},
	"冷却水入口温度":   {EN: "Cooling Water Inlet Temperature", JA: "冷却水入口温度"},
	"冷却水出口温度":   {EN: "Cooling Water Outlet Temperature", JA: "冷却水出口温度"},
	"加湿器水温":     {EN: "Humidifier Water Temperature", JA: "加湿器水温"},
	"加湿器给水口温度":  {EN: "Humidifier Water Inlet Temperature", JA: "加湿器給水口温度"},
	"再热器出口温度":   {EN: "Reheater Outlet Temperature", JA: "再熱器出口温度"},
	"干燥过滤器入口温度": {EN: "Drier Filter Inlet Temperature", JA: "ドライフィルター入口温度"},
	"干燥过滤器出口温度": {EN: "Drier Filter Outlet Temperature", JA: "ドライフィルター出口温度"},
	"风量":        {EN: "Air Volume", JA: "風量"},
	"进风风量":      {EN: "Inlet Air Volume", JA: "吸込風量"},
	"噪声":        {EN: "Noise", JA: "騒音"},
	"设备噪音":      {EN: "Equipment Noise", JA: "設備騒音"},
	"振动":        {EN: "Vibration", JA: "振動"},
	"震动位移":      {EN: "Vibration Displacement", JA: "振動変位"},
	"压力":        {EN: "Pressure", JA: "圧力"},
	"功率":        {EN: "Power", JA: "電力"},
	"电流":        {EN: "Current", JA: "電流"},
	"电压":        {EN: "Voltage", JA: "電圧"},
	"频率":        {EN: "Frequency", JA: "周波数"},
	"告警状态":      {EN: "Alarm Status", JA: "アラーム状態"},
	"通信状态":      {EN: "Communication Status", JA: "通信状態"},
	"启停控制":      {EN: "Start/Stop Control", JA: "起動停止制御"},
	"小数点位数":     {EN: "Decimal Places", JA: "小数点桁数"},
	"新建配置名":     {EN: "New Configuration Name", JA: "新規設定名"},
	"检测标准名":     {EN: "Detection Standard Name", JA: "検出基準名"},
	"S_表面积":     {EN: "Surface Area", JA: "表面積"},
}

var precisionACName = regexp.MustCompile(`^(\d+)号精密空调$`)
var camelBoundary = regexp.MustCompile(`([a-z0-9])([A-Z])`)
var acronymBoundary = regexp.MustCompile(`([A-Z]+)([A-Z][a-z])`)

func main() {
	configPath := flag.String("config", "backend/configs/config.json", "backend config path")
	dryRun := flag.Bool("dry-run", false, "print planned changes without writing")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatal(err)
	}
	db, err := database.Connect(cfg.Database)
	if err != nil {
		log.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		log.Fatal(err)
	}
	defer sqlDB.Close()

	var projectRows []projectRow
	if err := db.Table("sys_projects").
		Select("id, project_code, name, display_name, display_name_en, display_name_ja").
		Find(&projectRows).Error; err != nil {
		log.Fatal(err)
	}

	var tagRows []tagRow
	if err := db.Table("sys_tags").
		Select("var_id, var_name, display_name, display_name_en, display_name_ja").
		Find(&tagRows).Error; err != nil {
		log.Fatal(err)
	}

	projectsUpdated := 0
	for _, row := range projectRows {
		next := translateProject(row)
		if next.EN == "" || next.JA == "" {
			continue
		}
		updates := displayNameUpdates(row.DisplayName, row.DisplayNameEN, row.DisplayNameJA, next)
		if len(updates) == 0 {
			continue
		}
		projectsUpdated++
		fmt.Printf("project %s: en=%q ja=%q\n", row.ProjectCode, next.EN, next.JA)
		if !*dryRun {
			if err := db.Table("sys_projects").Where("id = ?", row.ID).Updates(updates).Error; err != nil {
				log.Fatal(err)
			}
		}
	}

	tagsUpdated := 0
	for _, row := range tagRows {
		next := translateTag(row)
		if next.EN == "" || next.JA == "" {
			continue
		}
		updates := displayNameUpdates(row.DisplayName, row.DisplayNameEN, row.DisplayNameJA, next)
		if len(updates) == 0 {
			continue
		}
		tagsUpdated++
		fmt.Printf("tag %d %s: en=%q ja=%q\n", row.VarID, row.VarName, next.EN, next.JA)
		if !*dryRun {
			if err := db.Table("sys_tags").Where("var_id = ?", row.VarID).Updates(updates).Error; err != nil {
				log.Fatal(err)
			}
		}
	}

	mode := "updated"
	if *dryRun {
		mode = "would update"
	}
	fmt.Printf("%s projects=%d tags=%d\n", mode, projectsUpdated, tagsUpdated)
}

func translateProject(row projectRow) translation {
	source := first(row.DisplayName, row.Name)
	if value, ok := zhTranslations[source]; ok {
		return value
	}
	matches := precisionACName.FindStringSubmatch(source)
	if len(matches) == 2 {
		return translation{EN: "Precision AC " + matches[1], JA: "精密空調" + matches[1] + "号機"}
	}
	if row.ProjectCode != "" {
		return translation{EN: row.ProjectCode, JA: row.ProjectCode}
	}
	return translation{}
}

func translateTag(row tagRow) translation {
	if value, ok := zhTranslations[row.DisplayName]; ok {
		return value
	}
	if value, ok := zhTranslations[row.VarName]; ok {
		return value
	}
	title := titleFromIdentifier(row.VarName)
	if title == "" {
		title = first(row.DisplayName, row.VarName)
	}
	return translation{EN: title, JA: title}
}

func displayNameUpdates(sourceZH string, currentEN string, currentJA string, next translation) map[string]any {
	updates := make(map[string]any)
	if shouldReplaceEnglish(currentEN, next) && next.EN != "" {
		updates["display_name_en"] = next.EN
	}
	if shouldReplaceJapanese(sourceZH, currentJA, next) && next.JA != "" {
		updates["display_name_ja"] = next.JA
	}
	return updates
}

func shouldReplaceEnglish(value string, next translation) bool {
	value = strings.TrimSpace(value)
	return value != next.EN && (value == "" || containsCJK(value))
}

func shouldReplaceJapanese(sourceZH string, value string, next translation) bool {
	sourceZH = strings.TrimSpace(sourceZH)
	value = strings.TrimSpace(value)
	return value != next.JA && (value == "" || value == next.EN || value == sourceZH)
}

func containsCJK(value string) bool {
	for _, r := range value {
		if r >= '\u4e00' && r <= '\u9fff' {
			return true
		}
	}
	return false
}

func titleFromIdentifier(value string) string {
	value = strings.TrimPrefix(value, "$")
	value = acronymBoundary.ReplaceAllString(value, `${1} ${2}`)
	value = camelBoundary.ReplaceAllString(value, `${1} ${2}`)
	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '_' || r == '-' || r == '.' || r == '/' || unicode.IsSpace(r)
	})
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		if isAcronym(part) {
			out = append(out, strings.ToUpper(part))
			continue
		}
		runes := []rune(strings.ToLower(part))
		runes[0] = unicode.ToUpper(runes[0])
		out = append(out, string(runes))
	}
	return strings.ReplaceAll(strings.Join(out, " "), " Of ", " ")
}

func isAcronym(value string) bool {
	hasLetter := false
	for _, r := range value {
		if unicode.IsLetter(r) {
			hasLetter = true
			if unicode.IsLower(r) {
				return false
			}
		}
	}
	return hasLetter
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func init() {
	for i := 1; i <= 12; i++ {
		zh := strconv.Itoa(i) + "号精密空调"
		if _, ok := zhTranslations[zh]; !ok {
			zhTranslations[zh] = translation{EN: fmt.Sprintf("Precision AC %d", i), JA: fmt.Sprintf("精密空調%d号機", i)}
		}
	}
}
