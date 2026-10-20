package protocols

import (
	"strconv"
	"strings"

	"github.com/acdc-awa/xpanel/internal/contracts"
	"github.com/acdc-awa/xpanel/internal/models"
)

// 规则求值（S4）：把字段声明里的 requires / forbids / allows 应用到真实入站上。
// 术语：
//   - document：入站三层 JSON + 列字段拍平成「规范路径 → 值」；
//   - present：路径存在且至少有一个非零值（空对象/空数组/false/0 视为不存在）；
//   - 容器路径（如 stream.tlsSettings）以任一子路径存在视为存在。

// ValidateInboundFields 按协议字段声明校验入站，返回结构化问题（空 = 通过）。
// 未声明协议返回 nil（Fail-open：协议白名单由 API 层单独把关）。
func ValidateInboundFields(inb *models.Inbound) []contracts.ValidationIssue {
	fs, ok := FieldSet(inb.Protocol)
	if !ok {
		return nil
	}
	doc := fieldDocument(inb)
	var issues []contracts.ValidationIssue
	for _, r := range fs.Rules {
		if !conditionsHold(doc, r.When) {
			continue
		}
		switch r.Op {
		case contracts.RuleRequires:
			if !present(doc, r.Target) {
				issues = append(issues, contracts.ValidationIssue{
					Path: r.Target, Code: "required",
					Params:  map[string]any{"protocol": inb.Protocol},
					Message: r.Reason,
				})
			}
		case contracts.RuleForbids:
			if present(doc, r.Target) {
				issues = append(issues, contracts.ValidationIssue{
					Path: r.Target, Code: "forbidden",
					Params:  map[string]any{"protocol": inb.Protocol},
					Message: r.Reason,
				})
			}
		case contracts.RuleAllows:
			if v, exists := doc[r.Target]; exists && !valueAllowed(v, r.Values) {
				actual, _ := scalarString(v)
				issues = append(issues, contracts.ValidationIssue{
					Path: r.Target, Code: "not_allowed",
					Params:  map[string]any{"allowed": r.Values, "actual": actual},
					Message: r.Reason,
				})
			}
		}
	}
	return issues
}

// ExplainInbound 返回只读预览的字段来源：声明字段逐个说明，存储里多出来的
// 未声明字段以 Declared=false 列出（"无法解释的字段"）。
func ExplainInbound(inb *models.Inbound) []contracts.ProvenanceEntry {
	fs, ok := FieldSet(inb.Protocol)
	if !ok {
		return nil
	}
	doc := fieldDocument(inb)

	entries := make([]contracts.ProvenanceEntry, 0, len(fs.Fields))
	covered := make(map[string]bool, len(fs.Fields))
	for _, f := range fs.Fields {
		covered[f.Path] = true
		entries = append(entries, contracts.ProvenanceEntry{
			Path:     f.Path,
			Label:    f.Label,
			Present:  present(doc, f.Path),
			Declared: true,
			Origin:   f.Origin,
			Role:     f.Role,
			Value:    doc[f.Path],
			Note:     f.Note,
		})
	}

	// 未声明字段：只列叶路径，避免容器噪音；根层容器本身不算。
	for path, val := range doc {
		if path == "" || covered[path] {
			continue
		}
		if !isLeaf(val) {
			continue
		}
		if coveredByDeclared(fs.Fields, path) {
			continue
		}
		entries = append(entries, contracts.ProvenanceEntry{
			Path:     path,
			Present:  !emptyValue(val),
			Declared: false,
			Value:    val,
			Note:     "未在字段声明中登记（界面不可见 / 生成器不解释）",
		})
	}
	return entries
}

func fieldDocument(inb *models.Inbound) map[string]any {
	spec := contracts.DecodeInbound(inb)
	doc := map[string]any{
		"settings": spec.Settings,
		"stream":   normalizeStreamAliases(spec.Stream),
		"sniffing": spec.Sniffing,
		"flow":     inb.Flow,
		"endpoint": map[string]any{"listen": inb.Listen, "port": inb.Port},
		"target": map[string]any{
			"mode":      targetMode(inb),
			"inboundId": inb.TargetInboundID,
			"address":   inb.TargetAddress,
			"port":      inb.TargetPort,
		},
	}
	flat := make(map[string]any)
	flattenPath("", doc, flat)
	return flat
}

// normalizeStreamAliases 让别名键落到规范键上（只读，不修改入参）。
func normalizeStreamAliases(stream map[string]any) map[string]any {
	if stream == nil {
		return nil
	}
	out := make(map[string]any, len(stream))
	for k, v := range stream {
		out[k] = v
	}
	if _, ok := out["xhttpSettings"]; !ok {
		if v, ok := out["splithttpSettings"]; ok {
			out["xhttpSettings"] = v
		}
	}
	if _, ok := out["tcpSettings"]; !ok {
		if v, ok := out["rawSettings"]; ok {
			out["tcpSettings"] = v
		}
	}
	return out
}

func targetMode(inb *models.Inbound) string {
	if inb.TargetInboundID != nil && *inb.TargetInboundID > 0 {
		return "linked"
	}
	if inb.TargetAddress != "" && inb.TargetPort > 0 {
		return "external"
	}
	return ""
}

func flattenPath(prefix string, v any, out map[string]any) {
	if prefix != "" {
		out[prefix] = v
	}
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	for k, child := range m {
		next := k
		if prefix != "" {
			next = prefix + "." + k
		}
		flattenPath(next, child, out)
	}
}

func conditionsHold(doc map[string]any, when []contracts.Condition) bool {
	for _, c := range when {
		v, exists := doc[c.Path]
		switch c.Op {
		case contracts.CondEq:
			if !exists || !equalValue(v, c.Value) {
				return false
			}
		case contracts.CondNe:
			if exists && equalValue(v, c.Value) {
				return false
			}
		case contracts.CondIn:
			if !exists || !valueAllowed(v, c.Values) {
				return false
			}
		case contracts.CondEmpty:
			if present(doc, c.Path) {
				return false
			}
		case contracts.CondNonEmpty:
			if !present(doc, c.Path) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// present 报告路径是否存在且非空；容器路径以任一非空子路径为准。
func present(doc map[string]any, path string) bool {
	if v, ok := doc[path]; ok && !emptyValue(v) {
		return true
	}
	prefix := path + "."
	for k, v := range doc {
		if strings.HasPrefix(k, prefix) && !emptyValue(v) {
			return true
		}
	}
	return false
}

func emptyValue(v any) bool {
	switch t := v.(type) {
	case nil:
		return true
	case string:
		return t == ""
	case bool:
		return !t
	case float64:
		return t == 0
	case int:
		return t == 0
	case uint64:
		return t == 0
	case *uint64:
		return t == nil || *t == 0
	case []any:
		return len(t) == 0
	case []string:
		return len(t) == 0
	case map[string]any:
		return len(t) == 0
	default:
		return false
	}
}

func isLeaf(v any) bool {
	_, isMap := v.(map[string]any)
	return !isMap
}

func coveredByDeclared(fields []contracts.FieldDecl, path string) bool {
	for _, f := range fields {
		if f.Path == path || strings.HasPrefix(f.Path, path+".") {
			return true
		}
	}
	return false
}

func equalValue(a, b any) bool {
	if sa, ok := scalarString(a); ok {
		if sb, ok := scalarString(b); ok {
			return sa == sb
		}
	}
	return false
}

func valueAllowed(v any, allowed []string) bool {
	s, ok := scalarString(v)
	if !ok {
		return false
	}
	for _, a := range allowed {
		if s == a {
			return true
		}
	}
	return false
}

func scalarString(v any) (string, bool) {
	switch t := v.(type) {
	case string:
		return t, true
	case bool:
		return strconv.FormatBool(t), true
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64), true
	case int:
		return strconv.Itoa(t), true
	case int64:
		return strconv.FormatInt(t, 10), true
	case uint64:
		return strconv.FormatUint(t, 10), true
	case *uint64:
		if t == nil {
			return "", true
		}
		return strconv.FormatUint(*t, 10), true
	default:
		return "", false
	}
}
