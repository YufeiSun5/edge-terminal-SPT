package database

import (
	"errors"
	"fmt"

	"gorm.io/gorm"
)

const defaultNodeIDBlockSize uint64 = 1000000000000

type IDGenerator struct {
	nodeID    uint64
	blockSize uint64
}

func NewIDGenerator(nodeID uint64, blockSize uint64) *IDGenerator {
	if nodeID == 0 {
		nodeID = 1
	}
	if blockSize == 0 {
		blockSize = defaultNodeIDBlockSize
	}
	return &IDGenerator{nodeID: nodeID, blockSize: blockSize}
}

func (g *IDGenerator) Next(tx *gorm.DB, table string) (uint64, error) {
	if g == nil {
		return 0, nil
	}
	base := g.nodeID * g.blockSize
	upper := base + g.blockSize
	var maxID uint64
	err := tx.Table(table).
		Select("COALESCE(MAX(id), 0)").
		Where("id >= ? AND id < ?", base, upper).
		Scan(&maxID).Error
	if err != nil {
		return 0, err
	}
	next := base + 1
	if maxID >= base {
		next = maxID + 1
	}
	if next >= upper {
		return 0, fmt.Errorf("id block exhausted for node %d table %s", g.nodeID, table)
	}
	return next, nil
}

func (r *Repository) nextID(tx *gorm.DB, table string) (uint64, error) {
	if r == nil || r.idGenerator == nil {
		return 0, nil
	}
	if tx == nil {
		return 0, errors.New("gorm transaction is required")
	}
	return r.idGenerator.Next(tx, table)
}

func (r *Repository) nextIDs(tx *gorm.DB, table string, count int) ([]uint64, error) {
	if count <= 0 {
		return nil, nil
	}
	first, err := r.nextID(tx, table)
	if err != nil || first == 0 {
		return nil, err
	}
	ids := make([]uint64, count)
	for i := range ids {
		ids[i] = first + uint64(i)
	}
	return ids, nil
}

func uintID(id uint64) uint {
	return uint(id)
}
