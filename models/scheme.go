package models

type Model struct {
	ID int `gorm:"primarykey" json:"Id"`
	//CreatedAt *time.Time   `json:"create_time"`
	//UpdatedAt *time.Time   `json:"update_time"`
	//DeletedAt sql.NullTime `gorm:"index" json:"-"`
}
