package biz

import (
	"testing"
	"time"

	"baboflow/internal/data/po"
)

func TestRetryBackoff_Exponential(t *testing.T) {
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{1, 5 * time.Second},
		{2, 15 * time.Second},
		{3, 45 * time.Second},
		{4, 135 * time.Second},
		{10, 5 * time.Minute}, // 封顶
	}
	for _, c := range cases {
		if got := retryBackoff(c.attempt); got != c.want {
			t.Errorf("retryBackoff(%d) = %v, want %v", c.attempt, got, c.want)
		}
	}
}

func TestApplyCronInput_Validation(t *testing.T) {
	// cron 缺表达式
	j := &po.CronJob{}
	if err := applyCronInput(j, &CronInput{TargetType: "chain", TargetID: "x", ScheduleType: "cron"}); err == nil {
		t.Fatal("cron without expr should fail")
	}
	// interval 缺秒数
	if err := applyCronInput(j, &CronInput{TargetType: "chain", TargetID: "x", ScheduleType: "interval"}); err == nil {
		t.Fatal("interval without intervalSec should fail")
	}
	// 非法 targetType
	if err := applyCronInput(j, &CronInput{TargetType: "http", TargetID: "x", ScheduleType: "interval", IntervalSec: 5}); err == nil {
		t.Fatal("bad targetType should fail")
	}
	// 合法 interval
	if err := applyCronInput(j, &CronInput{TargetType: "chain", TargetID: "x", ScheduleType: "interval", IntervalSec: 5}); err != nil {
		t.Fatalf("valid interval should pass, got %v", err)
	}
	if j.ScheduleType != "interval" || j.IntervalSec != 5 {
		t.Fatalf("fields not applied: %+v", j)
	}
}
