/**
 * 审计详情翻译层：把审计 detail 翻译为「一句话摘要 + 字段表 + 大文本预览」。
 *
 * 支持三种数据形态（历史数据共存，永远保留原文）：
 *  - v2 envelope：后端中间件落库的结构化信封 { v:2, method, path, params, body }，
 *    body 已脱敏、嵌套 JSON 已剥层、大文本已是 __text 摘要标记；
 *  - 旧版裸 JSON body（不可信 method/params，按 action 语义推断动词）；
 *  - 非 JSON 文本（手动打点的中文句子 / 旧版回退文本）→ 原样展示。
 */

export interface AuditField {
  label: string
  value: string
  mono?: boolean
}

export interface AuditTextChip {
  title: string
  meta: string
  preview: string
}

export interface AuditView {
  summary: string
  fields: AuditField[]
  texts: AuditTextChip[]
  raw: string
  /** v2 信封携带；旧数据为空串 */
  method: string
}

interface TextMark {
  __text: { preview: string; lines: number; chars: number }
}

interface Envelope {
  v?: number
  method?: string
  path?: string
  params?: Record<string, string>
  status?: number
  /** 目标名注册表预读的实体显示名（标题/邮箱/名称/标签） */
  target?: string
  body?: unknown
  body_raw?: string
  __raw?: string
}

type Obj = Record<string, unknown>

function isObj(v: unknown): v is Obj {
  return !!v && typeof v === 'object' && !Array.isArray(v)
}

function isTextMark(v: unknown): v is TextMark {
  return isObj(v) && isObj(v['__text'])
}

// ============================== 值格式化 ==============================

type Fmt = (v: unknown) => string

function money(v: unknown): string {
  const n = Number(v)
  return Number.isFinite(n) ? '¥' + (n / 100).toFixed(2) : String(v)
}

function signedMoney(v: unknown): string {
  const n = Number(v)
  if (!Number.isFinite(n)) return String(v)
  return (n >= 0 ? '+' : '') + '¥' + (n / 100).toFixed(2)
}

function gb(v: unknown): string {
  const n = Number(v)
  return Number.isFinite(n) ? `${n} GB` : String(v)
}

function days(v: unknown): string {
  return `${v} 天`
}

function seconds(v: unknown): string {
  return `${v} 秒`
}

function boolFmt(v: unknown): string {
  return v === true || v === 1 || v === '1' ? '是' : '否'
}

function onOff(v: unknown): string {
  return v === true || v === 1 || v === '1' ? '开启' : '关闭'
}

function yesNoEnable(on: string, off: string): Fmt {
  return (v) => (v === true || v === 1 || v === '1' ? on : off)
}

function dateOnly(v: unknown): string {
  const s = String(v)
  return s.length >= 10 ? s.slice(0, 10) : s
}

function hashRef(v: unknown): string {
  return `#${v}`
}

const PROTOCOL_LABELS: Record<string, string> = {
  vless: 'VLESS',
  vmess: 'VMess',
  trojan: 'Trojan',
  shadowsocks: 'Shadowsocks',
  none: '无',
  tls: 'TLS',
  reality: 'REALITY',
  tcp: 'TCP',
  ws: 'WebSocket',
  grpc: 'gRPC',
  httpupgrade: 'HTTPUpgrade',
  xhttp: 'XHTTP',
  kcp: 'mKCP',
  quic: 'QUIC',
}

function enumLabel(v: unknown): string {
  const s = String(v)
  return PROTOCOL_LABELS[s.toLowerCase()] ?? s
}

function deviceLimit(v: unknown): string {
  const n = Number(v)
  if (!Number.isFinite(n) || n <= 0) return '不限'
  return `${n} 台`
}

function groupRef(v: unknown): string {
  const n = Number(v)
  if (!Number.isFinite(n) || n <= 0) return '不绑定'
  return `#${n}`
}

function arrayCount(v: unknown): string {
  return Array.isArray(v) ? `${v.length} 个` : String(v)
}

// ============================== 基础渲染 ==============================

interface FieldDef {
  label: string
  fmt?: Fmt
  /** 从 body 内嵌路径取值（stream_settings.network）；旧数据字符串会自动尝试 JSON 解析 */
  path?: string
  /** 等宽字体展示（域名/地址/端口等技术值） */
  mono?: boolean
  /** 仅用于排除剩余字段扫描（子字段已由 path 型条目表达） */
  hidden?: boolean
}

type Dict = Record<string, FieldDef>

interface Ctx {
  action: string
  method: string
  params: Record<string, string>
  /** 注册表预读的目标显示名（删除/启停等无 body 上下文操作的实体名兜底） */
  target: string
  body: Obj
  view: AuditView
}

function stringifyShort(v: unknown): string {
  if (typeof v === 'string') return v
  try {
    return JSON.stringify(v) ?? String(v)
  } catch {
    return String(v)
  }
}

function truncate(s: string, n: number): string {
  return s.length > n ? s.slice(0, n) + '…' : s
}

function metaOf(lines: number, chars: number): string {
  const size = chars >= 1024 ? (chars / 1024).toFixed(1) + ' KB' : `${chars} 字符`
  return lines > 1 ? `${lines} 行 / ${size}` : size
}

function chipFromString(s: string): TextMark['__text'] {
  const chars = s.length
  return { preview: truncate(s, 200), lines: s.split('\n').length, chars }
}

function pushChip(view: AuditView, title: string, mark: TextMark['__text']) {
  view.texts.push({ title, meta: metaOf(mark.lines, mark.chars), preview: mark.preview })
}

function pushValue(view: AuditView, label: string, v: unknown, fmt?: Fmt, mono?: boolean) {
  if (v === undefined || v === null || v === '') return
  if (isTextMark(v)) {
    pushChip(view, label, v.__text)
    return
  }
  if (typeof v === 'string' && (v.length > 256 || v.includes('\n'))) {
    // 长文本 / 多行文本（描述、正文、模板）转预览 chip，不塞字段行
    pushChip(view, label, chipFromString(v))
    return
  }
  const s = fmt ? fmt(v) : stringifyShort(v)
  if (s === '') return
  view.fields.push({ label, value: s, mono: mono ?? (!fmt && typeof v !== 'string') })
}

function pushDictFields(view: AuditView, obj: Obj, dict: Dict) {
  for (const [key, def] of Object.entries(dict)) {
    if (def.hidden) continue
    const v = def.path ? dig(obj, def.path) : obj[key]
    pushValue(view, def.label, v, def.fmt, def.mono)
  }
}

/** 剩余未识别字段兜底：mono 键名 + 截断值（大文本转 chip） */
function pushRemaining(ctx: Ctx, ...dicts: Dict[]) {
  const known = new Set(dicts.flatMap((d) => Object.keys(d)))
  for (const [k, v] of Object.entries(ctx.body)) {
    if (known.has(k) || v === undefined || v === null || v === '') continue
    pushValue(ctx.view, k, v, undefined, true)
  }
}

/** 沿路径取值；旧数据中间层是 JSON 字符串时自动解析 */
function dig(obj: Obj, path: string): unknown {
  let cur: unknown = obj
  for (const seg of path.split('.')) {
    if (typeof cur === 'string') {
      try {
        cur = JSON.parse(cur)
      } catch {
        return undefined
      }
    }
    if (!isObj(cur)) return undefined
    cur = cur[seg]
  }
  if (typeof cur === 'string') {
    try {
      const p = JSON.parse(cur)
      if (p && typeof p === 'object') cur = p
    } catch {
      /* 保持原值 */
    }
  }
  return cur
}

function targetId(ctx: Ctx): string {
  return ctx.params.id ? ` #${ctx.params.id}` : ''
}

function nameOf(ctx: Ctx, ...keys: string[]): string {
  for (const k of keys.length ? keys : ['name', 'tag', 'title', 'domain', 'email']) {
    const v = ctx.body[k]
    if (typeof v === 'string' && v) return `「${v}」`
  }
  // body 带不出名字（删除/启停/重置）→ 注册表预读的目标显示名
  if (ctx.target) return `「${ctx.target}」`
  return ''
}

function buildSummary(ctx: Ctx, verb: string, name = ''): string {
  return `${verb}${targetId(ctx)}${name}`
}

// ============================== 家族渲染器 ==============================

const plansDict: Dict = {
  name: { label: '名称' },
  price_cents: { label: '价格', fmt: money },
  traffic_gb: { label: '流量', fmt: gb },
  duration_days: { label: '有效期', fmt: days },
  device_limit: { label: '设备限制', fmt: deviceLimit },
  permission_group_id: { label: '绑定权限组', fmt: groupRef },
  sort_order: { label: '商城排序' },
  is_featured: { label: '热门推荐', fmt: boolFmt },
  purchasable: { label: '可新购', fmt: yesNoEnable('可购', '停售') },
  renewable: { label: '可续费', fmt: yesNoEnable('可续', '不可续') },
  sync_users: { label: '同步存量用户', fmt: boolFmt },
  description: { label: '套餐描述' },
}

function renderPlans(ctx: Ctx) {
  const nm = nameOf(ctx)
  if (ctx.method === 'DELETE') ctx.view.summary = buildSummary(ctx, '删除套餐', nameOf(ctx))
  else if (ctx.method === 'POST') ctx.view.summary = buildSummary(ctx, '创建套餐', nm)
  else ctx.view.summary = buildSummary(ctx, '修改套餐', nm)
  if (ctx.method === 'DELETE') return
  pushDictFields(ctx.view, ctx.body, plansDict)
  pushRemaining(ctx, plansDict)
}

function renderGiftCards(ctx: Ctx) {
  if (ctx.method === 'DELETE') {
    ctx.view.summary = buildSummary(ctx, '删除礼品卡')
    return
  }
  const count = Number(ctx.body['count']) || 0
  const face = money(ctx.body['face_value_cents'])
  ctx.view.summary = `生成礼品卡 ×${count}（面值 ${face}）`
  pushDictFields(ctx.view, ctx.body, {
    count: { label: '数量' },
    name: { label: '名称' },
    face_value_cents: { label: '面值', fmt: money },
    expires_at: { label: '有效期至', fmt: dateOnly },
  })
  pushRemaining(ctx)
}

function renderInvitations(ctx: Ctx) {
  if (ctx.method === 'DELETE') {
    ctx.view.summary = buildSummary(ctx, '作废邀请码', nameOf(ctx, 'code'))
    return
  }
  const count = Number(ctx.body['count']) || 0
  ctx.view.summary = `批量生成邀请码 ×${count}`
  pushDictFields(ctx.view, ctx.body, {
    count: { label: '数量' },
    expires: { label: '有效期至', fmt: dateOnly },
  })
}

const usersDict: Dict = {
  email: { label: '邮箱' },
  password: { label: '密码' },
  remark: { label: '备注' },
  balance_cents: { label: '余额', fmt: money },
}

function renderUsers(ctx: Ctx) {
  const a = ctx.action
  if (a.endsWith('.toggle')) ctx.view.summary = buildSummary(ctx, '切换用户启停状态', nameOf(ctx))
  else if (a.endsWith('.2fa.disable')) ctx.view.summary = buildSummary(ctx, '关闭用户两步验证', nameOf(ctx))
  else if (a.endsWith('.reset-traffic')) ctx.view.summary = buildSummary(ctx, '重置用户流量', nameOf(ctx))
  else if (a.endsWith('.balance')) {
    ctx.view.summary = buildSummary(ctx, '调整用户余额', nameOf(ctx))
    pushDictFields(ctx.view, ctx.body, {
      amount_cents: { label: '金额', fmt: signedMoney },
      remark: { label: '备注' },
    })
    return
  } else if (a.endsWith('.subscribe-token.reset')) {
    ctx.view.summary = buildSummary(ctx, '重置订阅 Token', nameOf(ctx))
    return
  } else if (ctx.method === 'DELETE') ctx.view.summary = buildSummary(ctx, '删除用户', nameOf(ctx))
  else if (ctx.method === 'POST') ctx.view.summary = buildSummary(ctx, '创建用户', nameOf(ctx, 'email'))
  else ctx.view.summary = buildSummary(ctx, '修改用户', nameOf(ctx, 'email'))
  pushDictFields(ctx.view, ctx.body, usersDict)
  pushRemaining(ctx, usersDict)
}

const COMMAND_LABELS: Record<string, string> = {
  push_config: '推送节点配置',
  restart_xray: '重启 Xray',
  upgrade_agent: '升级 Agent',
  get_status: '查询节点状态',
  get_logs: '查看节点日志',
}

const serversDict: Dict = {
  server_type: { label: '类型', fmt: enumLabel },
  name: { label: '名称' },
  host: { label: '地址', mono: true },
  location: { label: '地区' },
  remark: { label: '备注' },
}

const routingDict: Dict = {
  outbound_tag: { label: '出站' },
  domain: { label: '域名' },
  ip: { label: 'IP' },
  port: { label: '端口' },
  network: { label: '网络' },
  protocol: { label: '协议' },
  inbound_tag: { label: '入站' },
  priority: { label: '优先级' },
  enabled: { label: '启用', fmt: onOff },
  remark: { label: '备注' },
  rule_json: { label: '', hidden: true },
}

function renderServers(ctx: Ctx) {
  const a = ctx.action
  if (a.endsWith('.command')) {
    const type = String(ctx.body['type'] ?? '')
    ctx.view.summary = buildSummary(ctx, `节点指令：${(COMMAND_LABELS[type] ?? type) || '未知'}`, nameOf(ctx))
    return
  }
  if (a.endsWith('.reset-secret')) {
    ctx.view.summary = buildSummary(ctx, '重置节点密钥')
    return
  }
  if (a.endsWith('.generate-config')) {
    ctx.view.summary = buildSummary(ctx, '重新生成节点配置')
    return
  }
  const del = ctx.method === 'DELETE'
  if (a.includes('.outbounds')) {
    ctx.view.summary = buildSummary(ctx, (del ? '删除' : '配置') + '节点出站')
  } else if (a.includes('.routing')) {
    ctx.view.summary = buildSummary(ctx, (del ? '删除' : '配置') + '节点分流规则')
  } else if (a.includes('.layers')) {
    ctx.view.summary = buildSummary(ctx, (del ? '删除' : '配置') + '分层规则')
  } else if (del) {
    ctx.view.summary = buildSummary(ctx, '删除节点', nameOf(ctx))
  } else if (ctx.method === 'POST') {
    ctx.view.summary = buildSummary(ctx, '创建节点', nameOf(ctx))
  } else {
    ctx.view.summary = buildSummary(ctx, '修改节点', nameOf(ctx))
  }
  if (del) return
  if (a.includes('.routing')) {
    pushDictFields(ctx.view, ctx.body, routingDict)
    pushRemaining(ctx, routingDict)
  } else {
    pushDictFields(ctx.view, ctx.body, serversDict)
    pushRemaining(ctx, serversDict)
  }
}

const inboundsDict: Dict = {
  tag: { label: '标签' },
  protocol: { label: '协议', fmt: enumLabel },
  port: { label: '端口' },
  listen: { label: '监听', mono: true },
  server_id: { label: '节点', fmt: hashRef },
  network: { label: '传输', path: 'stream_settings.network', fmt: enumLabel },
  security: { label: '安全', path: 'stream_settings.security', fmt: enumLabel },
  fingerprint: { label: '指纹', path: 'stream_settings.fingerprint' },
  settings_json: { label: '', hidden: true },
  stream_settings: { label: '', hidden: true },
}

function renderInbounds(ctx: Ctx) {
  const a = ctx.action
  if (a.endsWith('.toggle')) {
    ctx.view.summary = buildSummary(ctx, '启停入站端口', nameOf(ctx))
    return
  }
  if (a.endsWith('.setup-internal') || a.endsWith('.rotate-internal')) {
    ctx.view.summary = buildSummary(ctx, '轮转内部中继账户', nameOf(ctx))
    return
  }
  const del = ctx.method === 'DELETE'
  if (del) ctx.view.summary = buildSummary(ctx, '删除入站', nameOf(ctx))
  else if (ctx.method === 'POST') ctx.view.summary = buildSummary(ctx, '新建入站', nameOf(ctx))
  else ctx.view.summary = buildSummary(ctx, '修改入站', nameOf(ctx))
  pushDictFields(ctx.view, ctx.body, inboundsDict)
  pushRemaining(ctx, inboundsDict)
}

const certsDict: Dict = {
  domain: { label: '域名', mono: true },
  cert_pem: { label: '证书内容' },
  key_pem: { label: '私钥' },
  remark: { label: '备注' },
}

function renderCerts(ctx: Ctx) {
  const a = ctx.action
  const domain = nameOf(ctx, 'domain')
  if (a.includes('self-signed')) {
    ctx.view.summary = `签发自签名证书${domain}`
    pushDictFields(ctx.view, ctx.body, { domain: certsDict['domain'] })
    return
  }
  const del = ctx.method === 'DELETE'
  if (del) ctx.view.summary = buildSummary(ctx, '删除证书', nameOf(ctx, 'domain'))
  else if (ctx.method === 'POST') ctx.view.summary = `上传证书${domain}`
  else ctx.view.summary = buildSummary(ctx, '更新证书', domain)
  if (!del) {
    pushDictFields(ctx.view, ctx.body, certsDict)
    pushRemaining(ctx, certsDict)
  }
}

const accessPointDict: Dict = {
  name: { label: '名称' },
  custom_host: { label: '自定义域名', mono: true },
  custom_port: { label: '自定义端口' },
  target_type: { label: '目标类型', fmt: (v) => (v === 'inbound' ? '入站' : String(v)) },
  target_inbound_id: { label: '目标入站', fmt: hashRef },
  enabled: { label: '启用', fmt: onOff },
  remark: { label: '备注' },
  permission_group_ids: { label: '授权权限组', fmt: arrayCount },
}

function renderAccessPoints(ctx: Ctx) {
  const a = ctx.action
  if (a.endsWith('.target')) {
    ctx.view.summary = buildSummary(ctx, '设置接入点目标')
    pushDictFields(ctx.view, ctx.body, accessPointDict)
    pushRemaining(ctx, accessPointDict)
    return
  }
  const del = ctx.method === 'DELETE'
  if (del) ctx.view.summary = buildSummary(ctx, '删除接入点', nameOf(ctx))
  else if (ctx.method === 'POST') ctx.view.summary = buildSummary(ctx, '新建接入点', nameOf(ctx))
  else ctx.view.summary = buildSummary(ctx, '修改接入点', nameOf(ctx))
  if (!del) {
    pushDictFields(ctx.view, ctx.body, accessPointDict)
    pushRemaining(ctx, accessPointDict)
  }
}

const permissionGroupDict: Dict = {
  name: { label: '名称' },
  remark: { label: '备注' },
  clash_template: { label: '订阅模板' },
  access_point_ids: { label: '接入点', fmt: arrayCount, hidden: true },
}

function renderPermissionGroups(ctx: Ctx) {
  const a = ctx.action
  if (a.endsWith('.access-points')) {
    const ids = ctx.body['access_point_ids']
    ctx.view.summary = buildSummary(ctx, '设置权限组接入点', Array.isArray(ids) ? `（${ids.length} 个）` : '')
    return
  }
  if (a.endsWith('.preview-template')) {
    ctx.view.summary = buildSummary(ctx, '预览权限组订阅模板')
    return
  }
  const del = ctx.method === 'DELETE'
  const nm = nameOf(ctx)
  const hasTpl = 'clash_template' in ctx.body
  const hasRest = Object.keys(ctx.body).some((k) => k !== 'clash_template')
  if (del) ctx.view.summary = buildSummary(ctx, '删除权限组', nameOf(ctx))
  else if (ctx.method === 'POST') ctx.view.summary = buildSummary(ctx, '新建权限组', nm)
  else if (hasTpl && !hasRest) ctx.view.summary = buildSummary(ctx, '更新订阅模板')
  else if (hasTpl) ctx.view.summary = buildSummary(ctx, '修改权限组', nm + '（含订阅模板）')
  else ctx.view.summary = buildSummary(ctx, '修改权限组', nm)
  if (!del) {
    pushDictFields(ctx.view, ctx.body, permissionGroupDict)
    pushRemaining(ctx, permissionGroupDict)
  }
}

const siteDict: Dict = {
  app_name: { label: '系统标题' },
  app_description: { label: '站点描述' },
  logo: { label: 'LOGO 地址', mono: true },
  favicon: { label: 'Favicon 地址', mono: true },
  subscribe_domain: { label: '订阅域名', mono: true },
  subscribe_url: { label: '订阅根地址', mono: true },
  subscribe_path: { label: '订阅入口路径', mono: true },
  sub_deny_code: { label: '无效订阅拒绝码' },
  sub_clean_ua: { label: '爬虫 UA 清洗', fmt: onOff },
  sub_strict_ua: { label: '严格客户端模式', fmt: onOff },
  sub_blocked_ua: { label: '封禁 UA 关键词' },
  tos_url: { label: '服务条款地址', mono: true },
  stop_register: { label: '关闭注册', fmt: onOff },
  currency: { label: '货币代码' },
  currency_symbol: { label: '货币符号' },
}

const captchaDict: Dict = {
  captcha_enable: { label: '人机验证', fmt: onOff },
  captcha_type: { label: '验证类型' },
  turnstile_site_key: { label: 'Turnstile Site Key', mono: true },
  turnstile_secret: { label: 'Turnstile 密钥' },
}

const agentDict: Dict = {
  agent_report_interval: { label: '流量上报周期', fmt: seconds },
  agent_heartbeat_interval: { label: '状态心跳周期', fmt: seconds },
}

function renderSettings(ctx: Ctx) {
  const groups: Array<[string, string, Dict]> = [
    ['site', '站点', siteDict],
    ['captcha', '人机验证', captchaDict],
    ['agent', '节点上报', agentDict],
  ]
  const present = groups.filter(([g]) => isObj(ctx.body[g]))
  const names = present.map(([, label]) => label).join(' / ')
  ctx.view.summary = `修改系统设置${names ? `（${names}）` : ''}`
  for (const [g, , dict] of present) pushDictFields(ctx.view, ctx.body[g] as Obj, dict)
}

const noticesDict: Dict = {
  title: { label: '标题' },
  status: { label: '启用', fmt: onOff },
  is_pinned: { label: '置顶', fmt: boolFmt },
  is_popup: { label: '弹窗', fmt: boolFmt },
  sort_order: { label: '排序' },
  content: { label: '公告正文' },
}

function renderNotices(ctx: Ctx) {
  const a = ctx.action
  if (a.endsWith('.toggle')) {
    const field = String(ctx.body['field'] ?? '')
    ctx.view.summary = buildSummary(ctx, '切换公告状态', nameOf(ctx, 'title') + (field ? `（${field}）` : ''))
    return
  }
  const del = ctx.method === 'DELETE'
  const nm = nameOf(ctx, 'title')
  if (del) ctx.view.summary = buildSummary(ctx, '删除公告', nm)
  else if (ctx.method === 'POST') ctx.view.summary = buildSummary(ctx, '发布公告', nm)
  else ctx.view.summary = buildSummary(ctx, '更新公告', nm)
  if (!del) {
    pushDictFields(ctx.view, ctx.body, noticesDict)
    pushRemaining(ctx, noticesDict)
  }
}

function renderTopology(ctx: Ctx) {
  ctx.view.summary = '保存拓扑布局'
  pushValue(ctx.view, '布局数据', ctx.body)
}

function renderBackup(ctx: Ctx) {
  ctx.view.summary = '创建系统备份'
}

function renderPanelUpdate(ctx: Ctx) {
  const version = typeof ctx.body['version'] === 'string' ? String(ctx.body['version']) : ''
  ctx.view.summary = `面板自更新${version ? `至 ${version}` : ''}`
  pushRemaining(ctx)
}

// ============================== 分发与入口 ==============================

function dispatch(ctx: Ctx) {
  const a = ctx.action
  if (a.startsWith('plans')) return renderPlans(ctx)
  if (a.startsWith('gift-cards')) return renderGiftCards(ctx)
  if (a.startsWith('invitations')) return renderInvitations(ctx)
  if (a.startsWith('users')) return renderUsers(ctx)
  if (a.startsWith('servers')) return renderServers(ctx)
  if (a.startsWith('inbounds')) return renderInbounds(ctx)
  if (a.startsWith('certs')) return renderCerts(ctx)
  if (a.startsWith('access-points')) return renderAccessPoints(ctx)
  if (a.startsWith('permission-groups')) return renderPermissionGroups(ctx)
  if (a.startsWith('settings')) return renderSettings(ctx)
  if (a.startsWith('notices') || a.startsWith('notice.')) return renderNotices(ctx)
  if (a.startsWith('topology')) return renderTopology(ctx)
  if (a.startsWith('backup')) return renderBackup(ctx)
  if (a.startsWith('update')) return renderPanelUpdate(ctx)
  return renderGeneric(ctx)
}

function renderGeneric(ctx: Ctx) {
  const id = targetId(ctx)
  ctx.view.summary = ctx.view.summary || `${ctx.action}${id}`
  pushRemaining(ctx)
}

export function renderAudit(action: string, detail: string): AuditView {
  const view: AuditView = { summary: '', fields: [], texts: [], raw: detail || '', method: '' }
  if (!detail) {
    view.summary = '—'
    return view
  }

  let parsed: unknown = null
  try {
    const p = JSON.parse(detail)
    if (p && typeof p === 'object') parsed = p
  } catch {
    parsed = null
  }

  // 非 JSON：手动打点的中文句子 / 旧版回退文本 → 原样展示
  if (!parsed) {
    view.summary = detail
    return view
  }

  const env = parsed as Envelope
  let params: Record<string, string> = {}
  let target = ''
  let body: unknown

  if (env.v === 2) {
    view.method = env.method || ''
    params = env.params || {}
    target = typeof env.target === 'string' ? env.target : ''
    if (typeof env.__raw === 'string') {
      view.summary = `${action}（详情过长，仅保留原文摘要）`
      view.fields.push({ label: '原文摘要', value: env.__raw, mono: true })
      return view
    }
    body = env.body
    if (typeof env.body_raw === 'string') {
      view.fields.push({ label: '请求体（非 JSON，已脱敏）', value: env.body_raw, mono: true })
    }
  } else {
    // 旧版裸 body：按 action 语义推断动词（根路径=创建，其余=修改）
    body = parsed
    view.method = action.indexOf('.') < 0 ? 'POST' : 'PUT'
  }

  const ctx: Ctx = {
    action,
    method: view.method,
    params,
    target,
    body: isObj(body) ? body : {},
    view,
  }
  dispatch(ctx)
  if (!view.summary) view.summary = `${action}${targetId(ctx)}`
  return view
}

// ============================== 操作元信息（分类标签 + 标题） ==============================

export interface ActionMeta {
  categoryName: string
  categoryColor: 'primary' | 'success' | 'warning' | 'danger' | 'info'
  title: string
}

export function getActionMeta(action: string, method = '', detail = ''): ActionMeta {
  const act = action || ''
  const d = detail || ''
  const isDel = method === 'DELETE' || d.includes('DELETE')

  // 1. 节点管理
  if (act.startsWith('servers')) {
    let title = '节点管理'
    if (act === 'servers') {
      title = '创建节点'
    } else if (act.endsWith('.command')) {
      if (d.includes('push_config')) title = '推送节点配置'
      else if (d.includes('restart_xray')) title = '重启节点 Xray'
      else if (d.includes('upgrade_agent')) title = '升级节点 Agent'
      else if (d.includes('get_status')) title = '查询节点状态'
      else if (d.includes('get_logs')) title = '查看节点日志'
      else title = '执行节点指令'
    } else if (act.endsWith('.reset-secret')) {
      title = '重置节点密钥'
    } else if (act.endsWith('.generate-config')) {
      title = '重新生成节点配置'
    } else if (act.includes('.outbounds')) {
      title = isDel ? '删除节点出站' : '配置节点出站'
    } else if (act.includes('.routing')) {
      title = isDel ? '删除节点分流' : '配置节点分流'
    } else if (act.includes('.layers')) {
      title = isDel ? '删除分层规则' : '配置分层规则'
    } else if (isDel) {
      title = '删除节点'
    } else {
      title = '修改节点信息'
    }
    return { categoryName: '节点', categoryColor: 'primary', title }
  }

  // 2. 用户管理
  if (act.startsWith('users') || act.startsWith('invitations')) {
    let title = '用户管理'
    if (act === 'users') {
      title = '创建新用户'
    } else if (act.endsWith('.toggle')) {
      title = '启停用户账号'
    } else if (act.endsWith('.2fa.disable')) {
      title = '关闭双重验证(2FA)'
    } else if (act.endsWith('.reset-traffic')) {
      title = '重置用户流量'
    } else if (act.endsWith('.balance')) {
      title = '调整用户余额'
    } else if (act.startsWith('invitations')) {
      title = isDel ? '作废邀请码' : '批量生成邀请码'
    } else if (act.endsWith('.subscribe-token/reset')) {
      title = '重置订阅 Token'
    } else if (isDel) {
      title = '删除用户账号'
    } else {
      title = '修改用户信息'
    }
    return { categoryName: '用户', categoryColor: 'success', title }
  }

  // 3. 财务与套餐
  if (act.startsWith('plans') || act.startsWith('orders') || act.startsWith('gift-cards') || act.startsWith('billing')) {
    let title = '财务套餐'
    if (act.startsWith('plans')) {
      title = act === 'plans' ? '创建套餐' : isDel ? '删除套餐' : '修改套餐'
    } else if (act.startsWith('orders')) {
      if (d.includes('confirm') || act.includes('confirm')) title = '确认订单'
      else if (d.includes('cancel') || act.includes('cancel')) title = '取消订单'
      else title = '订单处理'
    } else if (act.startsWith('gift-cards')) {
      title = isDel ? '删除礼品卡' : '批量生成礼品卡'
    }
    return { categoryName: '财务', categoryColor: 'warning', title }
  }

  // 4. 入站与证书
  if (act.startsWith('inbounds') || act.startsWith('certs') || act.startsWith('access-points') || act.startsWith('permission-groups')) {
    let title = '入站证书'
    if (act.startsWith('inbounds')) {
      if (act.endsWith('.toggle')) title = '启停入站端口'
      else if (act.includes('setup-internal') || act.includes('rotate-internal')) title = '轮转内部中继账户'
      else if (isDel) title = '删除入站端口'
      else title = act === 'inbounds' ? '新建入站端口' : '修改入站端口'
    } else if (act.startsWith('certs')) {
      if (act.includes('self-signed')) title = '签发自签名证书'
      else title = isDel ? '删除 TLS 证书' : '上传/配置 TLS 证书'
    } else if (act.startsWith('access-points')) {
      title = isDel ? '删除接入点' : '配置自定义接入点'
    } else if (act.startsWith('permission-groups')) {
      title = isDel ? '删除权限组' : '配置用户权限组'
    }
    return { categoryName: '协议证书', categoryColor: 'info', title }
  }

  // 5. 系统设置与运维
  if (act.startsWith('settings') || act.startsWith('notices') || act.startsWith('backup') || act.startsWith('topology') || act.startsWith('update')) {
    let title = '系统设置'
    if (act.startsWith('settings')) title = '修改全局系统设置'
    else if (act.startsWith('notices')) {
      if (act.endsWith('.toggle')) title = '切换公告状态'
      else title = isDel ? '删除系统公告' : '发布/编辑系统公告'
    } else if (act.startsWith('backup')) title = '创建系统数据备份'
    else if (act.startsWith('topology')) title = '保存拓扑结构布局'
    else if (act.startsWith('update')) title = '面板自更新'
    return { categoryName: '系统', categoryColor: 'danger', title }
  }

  // 6. 认证登录
  if (act.startsWith('auth')) {
    return {
      categoryName: '认证',
      categoryColor: 'info',
      title: act === 'auth.login' ? '账号登录' : '身份认证操作',
    }
  }

  return { categoryName: '操作', categoryColor: 'info', title: act }
}
