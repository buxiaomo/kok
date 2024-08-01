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

func ConnectDB(t, url string) {
	var (
		database *gorm.DB
		err      error
	)
	cfg := &gorm.Config{
		PrepareStmt: true,
		Logger: logger.New(
			log.New(os.Stdout, "\r\n", log.LstdFlags), // io writer
			logger.Config{
				SlowThreshold: time.Second, // 慢 SQL 阈值
				LogLevel:      logger.Info, // Log level
				Colorful:      true,        // 禁用彩色打印
			},
		),
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true, // 使用单数表名
		},
		NowFunc: func() time.Time {
			return time.Now().Local()
		},
	}

	switch t {
	case "mysql":
		database, err = gorm.Open(mysql.Open(url), cfg)
	case "postgres":
		database, err = gorm.Open(postgres.Open(url), cfg)
	case "sqlite":
		database, err = gorm.Open(sqlite.Open(url), cfg)
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
}

func Healthz() (err error) {
	sql, err := db.DB()
	if err != nil {
		return
	}
	return sql.Ping()
}
