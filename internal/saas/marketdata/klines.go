package marketdata

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"bian-trade-go/internal/saas/store"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	BinancePublicBaseURL = "https://api.binance.com"
	BinanceKLinesPath    = "/api/v3/klines"
	BinanceMaxKLineLimit = 1000
	MaxLeverage          = 100
)

type Range struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type CoverageReport struct {
	Symbol        string  `json:"symbol"`
	Interval      string  `json:"interval"`
	Start         int64   `json:"start_ms"`
	End           int64   `json:"end_ms"`
	ExpectedBars  int     `json:"expected_bars"`
	AvailableBars int     `json:"available_bars"`
	FetchedBars   int     `json:"fetched_bars"`
	MissingRanges []Range `json:"missing_ranges,omitempty"`
	Complete      bool    `json:"complete"`
}

type EnsureOptions struct {
	Symbol   string
	Interval string
	Start    time.Time
	End      time.Time
	Now      time.Time
	Client   *BinanceClient
}

type BinanceClient struct {
	BaseURL    string
	HTTPClient *http.Client
}

type BinanceKLine struct {
	Symbol    string
	Interval  string
	OpenTime  time.Time
	Open      string
	High      string
	Low       string
	Close     string
	Volume    string
	CloseTime time.Time
}

func NormalizeSymbol(symbol string) string {
	replacer := strings.NewReplacer("/", "", "-", "", "_", "")
	return strings.ToUpper(replacer.Replace(strings.TrimSpace(symbol)))
}

func NormalizeInterval(interval string) string {
	return strings.TrimSpace(strings.ToLower(interval))
}

func IntervalDuration(interval string) (time.Duration, bool) {
	switch NormalizeInterval(interval) {
	case "1m":
		return time.Minute, true
	case "5m":
		return 5 * time.Minute, true
	case "15m":
		return 15 * time.Minute, true
	case "1h":
		return time.Hour, true
	case "1d":
		return 24 * time.Hour, true
	default:
		return 0, false
	}
}

func ValidateLeverage(leverage int) (int, error) {
	if leverage == 0 {
		return 1, nil
	}
	if leverage < 1 {
		return 0, errors.New("leverage must be at least 1x")
	}
	if leverage > MaxLeverage {
		return 0, fmt.Errorf("leverage must not exceed %dx", MaxLeverage)
	}
	return leverage, nil
}

func EnsureClosedKLines(ctx context.Context, db *gorm.DB, opts EnsureOptions) (CoverageReport, error) {
	if db == nil {
		return CoverageReport{}, errors.New("ensure klines: database is nil")
	}
	symbol := NormalizeSymbol(opts.Symbol)
	interval := NormalizeInterval(opts.Interval)
	duration, ok := IntervalDuration(interval)
	if !ok {
		return CoverageReport{}, fmt.Errorf("unsupported kline interval %q", opts.Interval)
	}
	if symbol == "" {
		return CoverageReport{}, errors.New("symbol is required")
	}
	if opts.Start.IsZero() || opts.End.IsZero() {
		return CoverageReport{}, errors.New("start and end time are required")
	}
	start := opts.Start.UTC()
	end := opts.End.UTC()
	if !start.Before(end) {
		return CoverageReport{}, errors.New("start time must be before end time")
	}
	now := opts.Now.UTC()
	if now.IsZero() {
		now = time.Now().UTC()
	}
	latestClosed := floorTime(now.Add(-duration), duration)
	if end.After(latestClosed) {
		end = latestClosed
	}
	if start.After(end) {
		return CoverageReport{}, errors.New("requested range has no fully closed bars")
	}
	start = floorTime(start, duration)
	end = floorTime(end, duration)

	report, err := inspectCoverage(ctx, db, symbol, interval, start, end, duration)
	if err != nil {
		return CoverageReport{}, err
	}
	if len(report.MissingRanges) == 0 {
		report.Complete = true
		return report, nil
	}

	client := opts.Client
	if client == nil {
		client = NewBinanceClient()
	}
	fetched := 0
	for _, missing := range report.MissingRanges {
		written, err := fetchAndStoreRange(ctx, db, client, symbol, interval, missing, duration, now)
		if err != nil {
			return report, err
		}
		fetched += written
	}

	report, err = inspectCoverage(ctx, db, symbol, interval, start, end, duration)
	if err != nil {
		return CoverageReport{}, err
	}
	report.FetchedBars = fetched
	report.Complete = len(report.MissingRanges) == 0
	if !report.Complete {
		return report, fmt.Errorf("kline data remains incomplete for %s %s", symbol, interval)
	}
	return report, nil
}

func NewBinanceClient() *BinanceClient {
	return &BinanceClient{
		BaseURL:    BinancePublicBaseURL,
		HTTPClient: &http.Client{Timeout: 20 * time.Second},
	}
}

func (c *BinanceClient) FetchKLines(ctx context.Context, symbol, interval string, start, end time.Time, limit int) ([]BinanceKLine, error) {
	if c == nil {
		return nil, errors.New("binance client is nil")
	}
	if limit <= 0 || limit > BinanceMaxKLineLimit {
		limit = BinanceMaxKLineLimit
	}
	baseURL := strings.TrimRight(c.BaseURL, "/")
	if baseURL == "" {
		baseURL = BinancePublicBaseURL
	}
	endpoint, err := url.Parse(baseURL + BinanceKLinesPath)
	if err != nil {
		return nil, err
	}
	params := url.Values{}
	params.Set("symbol", NormalizeSymbol(symbol))
	params.Set("interval", NormalizeInterval(interval))
	params.Set("startTime", strconv.FormatInt(start.UTC().UnixMilli(), 10))
	params.Set("endTime", strconv.FormatInt(end.UTC().UnixMilli(), 10))
	params.Set("limit", strconv.Itoa(limit))
	endpoint.RawQuery = params.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, err
	}
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("binance klines returned HTTP %d: %s", resp.StatusCode, string(raw))
	}

	var payload [][]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode binance klines: %w", err)
	}
	rows := make([]BinanceKLine, 0, len(payload))
	for _, item := range payload {
		row, err := decodeBinanceKLine(symbol, interval, item)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func inspectCoverage(ctx context.Context, db *gorm.DB, symbol, interval string, start, end time.Time, duration time.Duration) (CoverageReport, error) {
	var rows []store.KLine
	if err := db.WithContext(ctx).
		Where("symbol = ? AND interval = ? AND open_time >= ? AND open_time <= ?", symbol, interval, start, end).
		Order("open_time ASC").
		Find(&rows).Error; err != nil {
		return CoverageReport{}, fmt.Errorf("inspect kline coverage: %w", err)
	}
	seen := make(map[int64]struct{}, len(rows))
	for _, row := range rows {
		seen[row.OpenTime.UTC().UnixMilli()] = struct{}{}
	}

	missing := make([]Range, 0)
	var current *Range
	expected := 0
	for cursor := floorTime(start, duration); !cursor.After(end); cursor = cursor.Add(duration) {
		expected++
		if _, ok := seen[cursor.UnixMilli()]; ok {
			if current != nil {
				missing = append(missing, *current)
				current = nil
			}
			continue
		}
		if current == nil {
			current = &Range{Start: cursor, End: cursor}
		} else {
			current.End = cursor
		}
	}
	if current != nil {
		missing = append(missing, *current)
	}
	return CoverageReport{
		Symbol:        symbol,
		Interval:      interval,
		Start:         floorTime(start, duration).UnixMilli(),
		End:           end.UnixMilli(),
		ExpectedBars:  expected,
		AvailableBars: len(seen),
		MissingRanges: missing,
		Complete:      len(missing) == 0,
	}, nil
}

func fetchAndStoreRange(ctx context.Context, db *gorm.DB, client *BinanceClient, symbol, interval string, missing Range, duration time.Duration, now time.Time) (int, error) {
	cursor := missing.Start.UTC()
	end := missing.End.UTC()
	total := 0
	for !cursor.After(end) {
		batchEnd := cursor.Add(time.Duration(BinanceMaxKLineLimit-1) * duration)
		if batchEnd.After(end) {
			batchEnd = end
		}
		rows, err := client.FetchKLines(ctx, symbol, interval, cursor, batchEnd, BinanceMaxKLineLimit)
		if err != nil {
			return total, fmt.Errorf("fetch binance klines %s %s: %w", symbol, interval, err)
		}
		if len(rows) == 0 {
			return total, fmt.Errorf("fetch binance klines %s %s: empty response for %s", symbol, interval, cursor.Format(time.RFC3339))
		}
		storeRows := make([]store.KLine, 0, len(rows))
		for _, row := range rows {
			if row.CloseTime.After(now) {
				continue
			}
			storeRows = append(storeRows, store.KLine{
				Symbol:    NormalizeSymbol(row.Symbol),
				Interval:  NormalizeInterval(row.Interval),
				OpenTime:  row.OpenTime.UTC(),
				Open:      store.Decimal(row.Open),
				High:      store.Decimal(row.High),
				Low:       store.Decimal(row.Low),
				Close:     store.Decimal(row.Close),
				Volume:    store.Decimal(row.Volume),
				CloseTime: row.CloseTime.UTC(),
			})
		}
		if len(storeRows) > 0 {
			if err := db.WithContext(ctx).Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "symbol"}, {Name: "interval"}, {Name: "open_time"}},
				DoUpdates: clause.AssignmentColumns([]string{"open", "high", "low", "close", "volume", "close_time", "updated_at"}),
			}).Create(&storeRows).Error; err != nil {
				return total, fmt.Errorf("store market klines: %w", err)
			}
			total += len(storeRows)
			cursor = storeRows[len(storeRows)-1].OpenTime.Add(duration)
			continue
		}
		cursor = cursor.Add(time.Duration(BinanceMaxKLineLimit) * duration)
	}
	return total, nil
}

func decodeBinanceKLine(symbol, interval string, item []any) (BinanceKLine, error) {
	if len(item) < 7 {
		return BinanceKLine{}, errors.New("binance kline payload has fewer than 7 fields")
	}
	openTime, err := anyMillis(item[0])
	if err != nil {
		return BinanceKLine{}, fmt.Errorf("parse open time: %w", err)
	}
	closeTime, err := anyMillis(item[6])
	if err != nil {
		return BinanceKLine{}, fmt.Errorf("parse close time: %w", err)
	}
	open, err := anyString(item[1])
	if err != nil {
		return BinanceKLine{}, fmt.Errorf("parse open: %w", err)
	}
	high, err := anyString(item[2])
	if err != nil {
		return BinanceKLine{}, fmt.Errorf("parse high: %w", err)
	}
	low, err := anyString(item[3])
	if err != nil {
		return BinanceKLine{}, fmt.Errorf("parse low: %w", err)
	}
	closePrice, err := anyString(item[4])
	if err != nil {
		return BinanceKLine{}, fmt.Errorf("parse close: %w", err)
	}
	volume, err := anyString(item[5])
	if err != nil {
		return BinanceKLine{}, fmt.Errorf("parse volume: %w", err)
	}
	return BinanceKLine{
		Symbol:    NormalizeSymbol(symbol),
		Interval:  NormalizeInterval(interval),
		OpenTime:  time.UnixMilli(openTime).UTC(),
		Open:      open,
		High:      high,
		Low:       low,
		Close:     closePrice,
		Volume:    volume,
		CloseTime: time.UnixMilli(closeTime).UTC(),
	}, nil
}

func anyMillis(value any) (int64, error) {
	switch v := value.(type) {
	case float64:
		return int64(v), nil
	case json.Number:
		return v.Int64()
	case string:
		return strconv.ParseInt(v, 10, 64)
	default:
		return 0, fmt.Errorf("unsupported millis type %T", value)
	}
}

func anyString(value any) (string, error) {
	switch v := value.(type) {
	case string:
		return v, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case json.Number:
		return v.String(), nil
	default:
		return "", fmt.Errorf("unsupported numeric string type %T", value)
	}
}

func floorTime(t time.Time, duration time.Duration) time.Time {
	if duration <= 0 {
		return t.UTC()
	}
	return time.Unix(0, t.UTC().UnixNano()).Truncate(duration)
}

func SortKLines(rows []store.KLine) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].OpenTime.Before(rows[j].OpenTime)
	})
}
