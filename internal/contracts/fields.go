package contracts

// 字段注册表（字段语义模型 S3）：把「界面能填什么、生成会写什么、各客户端方言叫什么」
// 收敛为一份纯数据契约。生成（Go）、校验（Go）、前端（TS）三个消费方读取同一份声明，
// 规则使用封闭词表（requires / forbids / allows），不自研表达式 DSL。

// FieldShape 字段形态。
type FieldShape string

const (
	ShapeScalar FieldShape = "scalar"
	ShapeEnum   FieldShape = "enum"
	ShapeArray  FieldShape = "array"
	ShapeObject FieldShape = "object"
	ShapeMap    FieldShape = "map"
)

// Cardinality 基数。
type Cardinality string

const (
	CardOne      Cardinality = "1"    // 恰好一个
	CardOptional Cardinality = "0..1" // 零或一个
	CardMany     Cardinality = "0..N" // 零或多个
)

// FieldOrigin 值由谁决定（与 FieldRole 正交）。
type FieldOrigin string

const (
	OriginAdmin   FieldOrigin = "admin"   // 管理员填写/选择
	OriginDerived FieldOrigin = "derived" // 由其他字段或面板策略派生
	OriginSystem  FieldOrigin = "system"  // 面板/内核固定，界面只读
)

// FieldDecl 单个字段的契约声明。
// Path 使用「层.键」点路径，与 InboundSpec 的三层结构对应：
// settings.* / stream.* / sniffing.* / target.*（直通目标）/ endpoint.*。
type FieldDecl struct {
	Path        string      `json:"path"`
	Label       string      `json:"label"` // 前端直接使用（本仓库无 i18n）
	Shape       FieldShape  `json:"shape"`
	Cardinality Cardinality `json:"cardinality"`
	Role        FieldRole   `json:"role"`   // server / client / both
	Origin      FieldOrigin `json:"origin"` // admin / derived / system
	Enum        []string    `json:"enum,omitempty"`
	Default     any         `json:"default,omitempty"`
	Required    bool        `json:"required,omitempty"`
	// Note 记录该字段的陷阱或口径（如「服务端数组、客户端标量」）。
	Note string `json:"note,omitempty"`
}

// CondOp 条件比较算子（封闭集合）。
type CondOp string

const (
	CondEq       CondOp = "eq"
	CondNe       CondOp = "ne"
	CondIn       CondOp = "in"
	CondEmpty    CondOp = "empty"
	CondNonEmpty CondOp = "nonEmpty"
)

// Condition 单个原子条件。复合条件由 Rule.When 的数组按 AND 组合。
type Condition struct {
	Path   string   `json:"path"`
	Op     CondOp   `json:"op"`
	Value  any      `json:"value,omitempty"`
	Values []string `json:"values,omitempty"`
}

// RuleOp 规则算子（封闭集合）。
type RuleOp string

const (
	RuleRequires RuleOp = "requires" // 目标字段必须存在/非空
	RuleForbids  RuleOp = "forbids"  // 目标字段必须不存在/为空
	RuleAllows   RuleOp = "allows"   // 目标字段取值必须在 Values 内
)

// Rule 交叉规则：When 全部成立时，Target 必须满足 Op。
// 例：{When:[network=xhttp, security=reality], Op:forbids, Target:stream.finalmask}
type Rule struct {
	When   []Condition `json:"when"`
	Op     RuleOp      `json:"op"`
	Target string      `json:"target"`
	Values []string    `json:"values,omitempty"`
	Reason string      `json:"reason"`
}

// DialectMapping 规范字段到各消费方方言的映射（一次语义、方言各译一次）。
// 空字符串表示该方言没有对应字段。
type DialectMapping struct {
	Path   string `json:"path"`
	Xray   string `json:"xray,omitempty"`   // 服务端配置 / xray 出站
	Mihomo string `json:"mihomo,omitempty"` // Clash YAML（mihomo）
	Link   string `json:"link,omitempty"`   // 订阅链接 query / 结构位置
	// Transform 基数或类型转换："" | "first" | "join" | "bool01" | "range" | "json"
	Transform string `json:"transform,omitempty"`
	Note      string `json:"note,omitempty"`
}

// ProtocolFieldSet 单个协议的完整声明。
type ProtocolFieldSet struct {
	Protocol string           `json:"protocol"`
	Label    string           `json:"label"`
	Fields   []FieldDecl      `json:"fields"`
	Rules    []Rule           `json:"rules"`
	Dialects []DialectMapping `json:"dialects"`
}

// ValidationIssue 结构化校验错误：前端按 Path 定位控件，Params 供文案插值。
type ValidationIssue struct {
	Path    string         `json:"path"`
	Code    string         `json:"code"` // required / forbidden / not_allowed
	Params  map[string]any `json:"params,omitempty"`
	Message string         `json:"message"`
}

// ProvenanceEntry 只读预览里的字段来源说明。
// Declared=false 表示该字段在生成/存储里出现但没有声明——"无法解释的字段"，
// 前端与测试应把它当成缺陷而不是正常状态。
type ProvenanceEntry struct {
	Path     string      `json:"path"`
	Label    string      `json:"label,omitempty"`
	Present  bool        `json:"present"`
	Declared bool        `json:"declared"`
	Origin   FieldOrigin `json:"origin,omitempty"`
	Role     FieldRole   `json:"role,omitempty"`
	Value    any         `json:"value,omitempty"`
	Note     string      `json:"note,omitempty"`
}
