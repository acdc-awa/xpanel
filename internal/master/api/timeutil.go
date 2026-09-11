package api

import "time"

// utcPtr 把可空时间归一为 UTC。
// 数据库统一以 UTC 存储：SQLite 把 time.Time 存成带偏移文本并按字面量比较，
// 若写入与查询的偏移不一致（如前端提交 +08:00），范围比较会得到错误结果。
func utcPtr(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	u := t.UTC()
	return &u
}
