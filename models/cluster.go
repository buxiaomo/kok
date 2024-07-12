package models

type Cluster struct {
	Model
	Namespace     string `gorm:"size:25;unique" json:"namespace" binding:"required"`
	Registry      string `gorm:"size:25" json:"registry" binding:"required"`
	Version       string `gorm:"size:25" json:"version" binding:"required"`
	ServiceSubnet string `gorm:"size:25" json:"serviceSubnet" binding:"required"`
	PodSubnet     string `gorm:"size:25" json:"PodSubnet" binding:"required"`
	DnsSvc        string `gorm:"size:25" json:"dnsSvc" binding:"required"`
	Network       string `gorm:"size:25" json:"network" binding:"required"`
	ExternalIp    string `gorm:"size:25" json:"externalIp" binding:"required"`
}
