package models

import (
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

type User struct {
	gorm.Model
	Fullname string `json:"full_name" gorm:"size:100"`
	Email    string `json:"email" gorm:"uniqueIndex;not null;size:100"`
	Password string `json:"-" gorm:"not null"`
	Role     string `json:"role" gorm:"default:user;size:20"`
	Active   bool   `json:"active" gorm:"default:true"`
}

// hashing the password (hook before the creating user)
func (u *User) BeforeCreate(tx *gorm.DB) error {
	if u.Password != "" {
		hashedPassword, err := bcrypt.GenerateFromPassword([]byte(u.Password), bcrypt.DefaultCost)
		if err != nil {
			return err
		}
		u.Password = string(hashedPassword)
	}
	return nil
}
