package models

import (
	"testing"
	"time"
)

// 周期起点恒对齐到 UTC 整点：这是「计费口径与明细小时分桶同轴」的前提。
func TestTrafficCycleAlign(t *testing.T) {
	sh, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name string
		in   time.Time
		want time.Time
	}{
		{"整点原样", time.Date(2026, 9, 17, 6, 0, 0, 0, time.UTC), time.Date(2026, 9, 17, 6, 0, 0, 0, time.UTC)},
		{"购买时刻（用户 23 实际值）", time.Date(2026, 9, 17, 6, 1, 21, 896970490, time.UTC), time.Date(2026, 9, 17, 6, 0, 0, 0, time.UTC)},
		{"小时最后一秒", time.Date(2026, 9, 17, 6, 59, 59, 999999999, time.UTC), time.Date(2026, 9, 17, 6, 0, 0, 0, time.UTC)},
		{"非 UTC 时区先转 UTC 再截断", time.Date(2026, 9, 17, 14, 1, 21, 0, sh), time.Date(2026, 9, 17, 6, 0, 0, 0, time.UTC)},
	}
	for _, c := range cases {
		if got := TrafficCycleAlign(c.in); !got.Equal(c.want) {
			t.Errorf("%s: TrafficCycleAlign(%v) = %v, want %v", c.name, c.in, got, c.want)
		}
		if got := TrafficCycleAlign(c.in); got.Location() != time.UTC {
			t.Errorf("%s: 结果时区 = %v, want UTC", c.name, got.Location())
		}
	}
}

// 桶起点文本必须与库内存储格式逐字节一致：SQLite 按文本字面量比较，
// 格式不一致（如 'T' 分隔、缺偏移）会让 period_start = ? 恒假，静默漏掉清零。
func TestTrafficCycleBucketFormat(t *testing.T) {
	got := TrafficCycleBucket(time.Date(2026, 9, 17, 6, 1, 21, 896970490, time.UTC))
	want := "2026-09-17 06:00:00+00:00"
	if got != want {
		t.Fatalf("TrafficCycleBucket = %q, want %q（须与 FormatDBTime 一致）", got, want)
	}
	if want != FormatDBTime(TrafficCycleAlign(time.Date(2026, 9, 17, 6, 1, 21, 0, time.UTC))) {
		t.Fatal("TrafficCycleBucket 与 FormatDBTime(TrafficCycleAlign(...)) 不一致")
	}
}
