package models

import (
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/gorm/schema"
	"log"
	"os"
	"time"
)

var db *gorm.DB

func ConnectDB(dbType, dsn string) {
	var (
		database *gorm.DB
		err      error
	)
	cfg := &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // 使用单数表名
		},
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
			logger.Config{
				SlowThreshold: time.Second, // 慢 SQL 阈值
				LogLevel:      logger.Info, // Log level
				Colorful:      true,        // 禁用彩色打印
			},
		),
		NowFunc: func() time.Time {
			return time.Now().Local()
		},
	}

	switch dbType {
	case "mysql":
		database, err = gorm.Open(mysql.Open(dsn), cfg)
	case "postgres":
		database, err = gorm.Open(postgres.Open(dsn), cfg)
	case "sqlite":
		database, err = gorm.Open(sqlite.Open(dsn), cfg)
	default:
		log.Fatal("db not support!")
	}

	if err != nil {
		log.Fatal(err.Error())
	}

	db = database

	if err := db.AutoMigrate(
		&Version{},
	); err != nil {
		log.Fatal(err.Error())
	}

	var v Version
	result := db.Find(&v)
	if result.RowsAffected == 0 {
		versions := []*Version{
			{Kubernetes: "v1.14.10", Etcd: "3.3.10-0", Containerd: "1.5.13", Runc: "1.1.2", Pause: "3.1", Coredns: "1.3.1", MetricsServer: "0.5.2", KubeStateMetrics: "1.9.8", Dashboard: "4.6.0"},
			{Kubernetes: "v1.15.12", Etcd: "3.3.10-0", Containerd: "1.5.13", Runc: "1.1.2", Pause: "3.1", Coredns: "1.3.1", MetricsServer: "0.5.2", KubeStateMetrics: "1.9.8", Dashboard: "4.6.0"},
			{Kubernetes: "v1.16.15", Etcd: "3.3.10-0", Containerd: "1.5.13", Runc: "1.1.2", Pause: "3.1", Coredns: "1.6.2", MetricsServer: "0.5.2", KubeStateMetrics: "1.9.8", Dashboard: "4.6.0"},
			{Kubernetes: "v1.17.17", Etcd: "3.4.3-0", Containerd: "1.5.13", Runc: "1.1.2", Pause: "3.1", Coredns: "1.6.5", MetricsServer: "0.5.2", KubeStateMetrics: "2.0.0", Dashboard: "4.6.0"},
			{Kubernetes: "v1.18.20", Etcd: "3.4.3-0", Containerd: "1.5.13", Runc: "1.1.2", Pause: "3.2", Coredns: "1.6.7", MetricsServer: "0.5.2", KubeStateMetrics: "2.0.0", Dashboard: "4.6.0"},
			{Kubernetes: "v1.19.16", Etcd: "3.4.13-0", Containerd: "1.5.13", Runc: "1.1.2", Pause: "3.2", Coredns: "1.7.0", MetricsServer: "0.5.2", KubeStateMetrics: "2.2.4", Dashboard: "6.0.7"},
			{Kubernetes: "v1.20.15", Etcd: "3.4.13-0", Containerd: "1.5.13", Runc: "1.1.2", Pause: "3.2", Coredns: "1.7.0", MetricsServer: "0.6.1", KubeStateMetrics: "2.2.4", Dashboard: "6.0.7"},
			{Kubernetes: "v1.21.14", Etcd: "3.4.13-0", Containerd: "1.5.13", Runc: "1.1.2", Pause: "3.4.1", Coredns: "1.8.0", MetricsServer: "0.6.1", KubeStateMetrics: "2.4.2", Dashboard: "7.8.0"},
			{Kubernetes: "v1.22.17", Etcd: "3.5.0-0", Containerd: "1.5.13", Runc: "1.1.2", Pause: "3.5", Coredns: "1.8.4", MetricsServer: "0.6.2", KubeStateMetrics: "2.6.0", Dashboard: "7.8.0"},
			{Kubernetes: "v1.23.17", Etcd: "3.5.1-0", Containerd: "1.7.0", Runc: "1.1.4", Pause: "3.6", Coredns: "1.8.6", MetricsServer: "0.6.3", KubeStateMetrics: "2.6.0", Dashboard: "7.8.0"},
			{Kubernetes: "v1.24.17", Etcd: "3.5.3-0", Containerd: "1.7.6", Runc: "1.1.9", Pause: "3.6", Coredns: "1.8.6", MetricsServer: "0.6.4", KubeStateMetrics: "2.6.0", Dashboard: "7.8.0"},
			{Kubernetes: "v1.25.16", Etcd: "3.5.4-0", Containerd: "1.7.13", Runc: "1.1.12", Pause: "3.8", Coredns: "1.9.3", MetricsServer: "0.6.4", KubeStateMetrics: "2.7.0", Dashboard: "7.8.0"},
			{Kubernetes: "v1.26.15", Etcd: "3.5.6-0", Containerd: "1.7.18", Runc: "1.1.12", Pause: "3.9", Coredns: "1.9.3", MetricsServer: "0.7.2", KubeStateMetrics: "2.9.2", Dashboard: "7.8.0"},
			{Kubernetes: "v1.27.16", Etcd: "3.5.7-0", Containerd: "2.0.0", Runc: "1.2.1", Pause: "3.9", Coredns: "1.10.1", MetricsServer: "0.7.2", KubeStateMetrics: "2.10.1", Dashboard: "7.8.0"},
			{Kubernetes: "v1.28.15", Etcd: "3.5.9-0", Containerd: "2.0.0", Runc: "1.2.1", Pause: "3.9", Coredns: "1.10.1", MetricsServer: "0.7.2", KubeStateMetrics: "2.11.0", Dashboard: "7.8.0"},
			{Kubernetes: "v1.29.13", Etcd: "3.5.12-0", Containerd: "2.0.0", Runc: "1.2.1", Pause: "3.9", Coredns: "1.11.1", MetricsServer: "0.7.2", KubeStateMetrics: "2.12.0", Dashboard: "7.8.0"},
			{Kubernetes: "v1.30.9", Etcd: "3.5.12-0", Containerd: "2.0.0", Runc: "1.2.1", Pause: "3.9", Coredns: "1.11.1", MetricsServer: "0.7.2", KubeStateMetrics: "2.13.0", Dashboard: "7.8.0"},
			{Kubernetes: "v1.31.5", Etcd: "3.5.15-0", Containerd: "2.0.0", Runc: "1.2.1", Pause: "3.10", Coredns: "1.11.1", MetricsServer: "0.7.2", KubeStateMetrics: "2.13.0", Dashboard: "7.8.0"},
			{Kubernetes: "v1.32.1", Etcd: "3.5.16-0", Containerd: "2.0.0", Runc: "1.2.1", Pause: "3.10", Coredns: "1.11.3", MetricsServer: "0.7.2", KubeStateMetrics: "2.13.0", Dashboard: "7.8.0"},
		}
		db.Create(versions)
	}
}

func Healthz() (err error) {
	sql, err := db.DB()
	if err != nil {
		return
	}
	return sql.Ping()
}
