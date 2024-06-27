package models

type Cluster struct {
	Model
	Namespace   string `gorm:"size:25;unique" json:"username" binding:"required"`
	Registry    string `gorm:"size:25" json:"registry" binding:"required"`
	Version     string `gorm:"size:25" json:"version" binding:"required"`
	ServiceCidr string `gorm:"size:25" json:"serviceCidr" binding:"required"`
	PodCidr     string `gorm:"size:25" json:"podCidr" binding:"required"`
	DnsSvc      string `gorm:"size:25" json:"dnsSvc" binding:"required"`
	Network     string `gorm:"size:25" json:"network" binding:"required"`
	ExternalIp  string `gorm:"size:25" json:"externalIp" binding:"required"`
}
