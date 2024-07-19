package models

type User struct {
	Model
	Username string `gorm:"size:25;unique" json:"username" binding:"required"`
	Password string `gorm:"size:25;unique" json:"password" binding:"required"`
}
