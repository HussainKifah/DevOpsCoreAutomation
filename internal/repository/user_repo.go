package repository

import (
	"errors"

	"github.com/Flafl/DevOpsCore/internal/models"
	"gorm.io/gorm"
)

type UserRepository interface {
	Create(user *models.User) error
	GetByFullName(fullName string) (*models.User, error)
	GetByID(id uint) (*models.User, error)
	GetByEmail(email string) (*models.User, error)
	Update(user *models.User) error
	Delete(id uint) error
	List(offset, limit int) ([]models.User, int64, error)
	Search(query string, offset, limit int) ([]models.User, int64, error)
}

type userRepository struct {
	db *gorm.DB
}

func NewUserRepository(db *gorm.DB) UserRepository {
	return &userRepository{db: db}
}

func (r *userRepository) Create(user *models.User) error {
	if err := r.db.Create(user).Error; err != nil {
		return err
	}
	return nil
}

func (r *userRepository) GetByID(id uint) (*models.User, error) {
	var user models.User
	err := r.db.Where("id = ?", id).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetByEmail(email string) (*models.User, error) {
	var user models.User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) GetByFullName(fullName string) (*models.User, error) {
	var user models.User
	err := r.db.Where("full_name = ?", fullName).First(&user).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("user not found")
		}
		return nil, err
	}
	return &user, nil
}

func (r *userRepository) Update(user *models.User) error {
	return r.db.Save(user).Error
}

func (r *userRepository) Delete(id uint) error {
	return r.db.Delete(&models.User{}, id).Error
}

func (r *userRepository) List(offset, limit int) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	//get total count
	if err := r.db.Model(&models.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// get paginated results
	err := r.db.Offset(offset).Limit(limit).Find(&users).Error
	return users, total, err
}

func (r *userRepository) Search(query string, offset, limit int) ([]models.User, int64, error) {
	var users []models.User
	var total int64

	searchQuery := "%" + query + "%"
	baseQuery := r.db.Where("full_name ILIKE ?", searchQuery)

	if err := baseQuery.Model(&models.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := baseQuery.Offset(offset).Limit(limit).Find(&users).Error
	return users, total, err
}
