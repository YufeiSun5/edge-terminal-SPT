package auth

import "gorm.io/gorm"

type UserStore struct {
	db *gorm.DB
}

func NewUserStore(db *gorm.DB) *UserStore {
	return &UserStore{db: db}
}

func (s *UserStore) FindUserByUsername(username string) (SysUser, error) {
	var user SysUser
	err := s.db.Where("username = ?", username).First(&user).Error
	return user, err
}

func (s *UserStore) FindUserByID(id uint) (SysUser, error) {
	var user SysUser
	err := s.db.Where("id = ?", id).First(&user).Error
	return user, err
}

func (s *UserStore) ListUsers() ([]SysUser, error) {
	var users []SysUser
	err := s.db.Order("id asc").Find(&users).Error
	return users, err
}
