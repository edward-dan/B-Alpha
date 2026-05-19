package store

import (
	"strings"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestKLineAutoMigrateCreatesConflictTargetIndex(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&KLine{}); err != nil {
		t.Fatalf("migrate kline: %v", err)
	}
	if !db.Migrator().HasIndex(&KLine{}, "idx_market_klines_symbol_interval_open_time_unique") {
		t.Fatal("expected kline unique index used by ON CONFLICT to be created")
	}

	openTime := time.Unix(1_700_000_000, 0).UTC()
	row := KLine{
		Symbol:    "BTCUSDT",
		Interval:  "1h",
		OpenTime:  openTime,
		Open:      Decimal("1"),
		High:      Decimal("2"),
		Low:       Decimal("1"),
		Close:     Decimal("2"),
		Volume:    Decimal("10"),
		CloseTime: openTime.Add(time.Hour),
	}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("insert first kline: %v", err)
	}
	duplicate := row
	duplicate.ID = 0
	if err := db.Create(&duplicate).Error; err == nil {
		t.Fatal("expected duplicate symbol/interval/open_time to fail")
	}
}
