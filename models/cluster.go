package models

type Version struct {
	Model
	Kubernetes       string `gorm:"size:25;unique" json:"Kubernetes" binding:"required"`
	Etcd             string `gorm:"size:25" json:"Etcd" binding:"required"`
	Containerd       string `gorm:"size:25" json:"Containerd" binding:"required"`
	Runc             string `gorm:"size:25" json:"Runc" binding:"required"`
	Pause            string `gorm:"size:25" json:"Pause" binding:"required"`
	Coredns          string `gorm:"size:25" json:"Coredns" binding:"required"`
	MetricsServer    string `gorm:"size:25" json:"MetricsServer" binding:"required"`
	KubeStateMetrics string `gorm:"size:25" json:"KubeStateMetrics" binding:"required"`
	Dashboard        string `gorm:"size:25" json:"Dashboard" binding:"required"`
}

func (t *Version) Select(k8s string) (v Version, err error) {
	err = db.Where("kubernetes = ?", k8s).First(&v).Error
	return
}

func (t *Version) SelectAll() (version []*Version, err error) {
	err = db.Find(&version).Error
	return
}

func (t *Version) Add(v Version) error {
	return db.Create(&v).Error
}

func (t *Version) Del(kubernetes string) error {
	return db.Where("kubernetes = ?", kubernetes).Delete(&t).Error
}
