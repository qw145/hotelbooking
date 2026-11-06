package helper

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/qw145/hotelbooking/model"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ErrUnsupportedDriver = errors.New("unsupported DB driver")
	Db                   *gorm.DB
)

func OpenDb(ctx context.Context) error {
	gormLog := logger.Default.LogMode(logger.Warn)

	if Cfg.Env == "development" {
		gormLog = logger.Default.LogMode(logger.Error)
	}

	gormCfg := &gorm.Config{
		Logger: gormLog,
		// GORM's default naming strategy pluralises table names, which is what
		// the migrations under migrations/ assume.
		NowFunc: func() time.Time { return time.Now().UTC() },
	}

	var dialector gorm.Dialector
	switch Cfg.Driver {
	case DriverSQLite:
		dialector = sqlite.Open(Cfg.DSN)
	default:
		return fmt.Errorf("unsupported DB driver %q", Cfg.Driver)
	}

	connectCtx, cancel := context.WithTimeout(ctx, time.Duration(Cfg.DBConnectTimeout))
	defer cancel()

	var err error
	Db, err = gorm.Open(dialector, gormCfg)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}

	// Take the pool from GORM rather than from dialectorFor: for SQL Server
	// the dialector creates and owns the pool itself, so it is only reachable
	// through db.DB().
	sqlDB, err := Db.DB()
	if err != nil {
		return fmt.Errorf("resolve connection pool: %w", err)
	}

	if err := sqlDB.PingContext(connectCtx); err != nil {
		return fmt.Errorf("ping database: %w", err)
	}

	if Cfg.AutoMigrate {
		Log.Info("running auto-migration", "driver", Cfg.Driver)
		if err := Db.WithContext(connectCtx).AutoMigrate(
			&model.Hotel{},
			&model.Room{},
			&model.Booking{},
		); err != nil {
			return fmt.Errorf("auto-migrate: %w", err)
		}
	}

	Log.Info("database connected", "driver", Cfg.Driver)

	return nil
}
