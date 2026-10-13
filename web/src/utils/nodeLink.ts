/**
 * nodeLink.ts - 节点链接（vless://, hysteria2://, socks5://, http:// 等）解析与格式化工具
 */

export interface ParsedNodeLink {
  protocol: 'vless' | 'hysteria' | 'socks' | 'http'
  tag?: string
  address: string
  port: number
  // Auth
  uuid?: string
  password?: string
  username?: string
  flow?: string
  encryption?: string
  // Transport
  network?: string
  wsPath?: string
  wsHost?: string
  xhttpMode?: string
  xhttpPath?: string
  xhttpHost?: string
  grpcServiceName?: string
  grpcAuthority?: string
  // Security
  security?: 'none' | 'tls' | 'reality'
  sni?: string
  allowInsecure?: boolean
  fingerprint?: string
  realityPublicKey?: string
  realityShortId?: string
}

export function parseNodeLink(raw: string): ParsedNodeLink {
  const trimmed = raw.trim()
  if (!trimmed) {
    throw new Error('链接不能为空')
  }

  let u: URL
  try {
    u = new URL(trimmed)
  } catch (err) {
    throw new Error(`无效的 URL 格式: ${String(err)}`)
  }

  const scheme = u.protocol.replace(':', '').toLowerCase()
  const tag = decodeURIComponent(u.hash ? u.hash.slice(1) : '')
  const address = u.hostname
  const port = u.port ? parseInt(u.port, 10) : 443

  if (!address) {
    throw new Error('链接缺少目标地址 (hostname)')
  }

  const q = u.searchParams

  if (scheme === 'vless') {
    const uuid = decodeURIComponent(u.username)
    if (!uuid) {
      throw new Error('vless 链接缺少 UUID')
    }

    const secRaw = (q.get('security') || 'none').toLowerCase()
    let security: 'none' | 'tls' | 'reality' = 'none'
    if (secRaw === 'reality') security = 'reality'
    else if (secRaw === 'tls') security = 'tls'

    const netRaw = (q.get('type') || 'tcp').toLowerCase()
    const pbk = q.get('pbk') || q.get('publicKey') || ''
    const sid = q.get('sid') || q.get('shortId') || ''
    const sni = q.get('sni') || q.get('serverName') || ''
    const fp = q.get('fp') || q.get('fingerprint') || 'chrome'
    const flow = q.get('flow') || ''
    const enc = q.get('encryption') || 'none'
    const allowInsecure = q.get('allowInsecure') === '1' || q.get('insecure') === '1'

    return {
      protocol: 'vless',
      tag,
      address,
      port,
      uuid,
      flow,
      encryption: enc,
      network: netRaw,
      security,
      sni,
      allowInsecure,
      fingerprint: fp,
      realityPublicKey: pbk,
      realityShortId: sid,
      wsPath: q.get('path') || '/',
      wsHost: q.get('host') || '',
      xhttpMode: q.get('mode') || 'auto',
      xhttpPath: q.get('path') || '/',
      xhttpHost: q.get('host') || '',
      grpcServiceName: q.get('serviceName') || 'grpc',
      grpcAuthority: q.get('authority') || '',
    }
  }

  if (scheme === 'hysteria2' || scheme === 'hy2' || scheme === 'hysteria') {
    const password = decodeURIComponent(u.username || u.password || '')
    if (!password) {
      throw new Error('hysteria2 链接缺少密码认证信息')
    }
    const sni = q.get('sni') || ''
    const allowInsecure = q.get('insecure') === '1' || q.get('allowInsecure') === '1'

    return {
      protocol: 'hysteria',
      tag,
      address,
      port,
      password,
      security: 'tls',
      sni,
      allowInsecure,
      fingerprint: q.get('fp') || q.get('fingerprint') || '',
    }
  }

  if (scheme === 'socks' || scheme === 'socks5' || scheme === 'socks5s') {
    const isTLS = scheme === 'socks5s' || q.get('tls') === '1' || q.get('security') === 'tls'
    const username = decodeURIComponent(u.username || '')
    const password = decodeURIComponent(u.password || '')
    return {
      protocol: 'socks',
      tag,
      address,
      port: u.port ? parseInt(u.port, 10) : 1080,
      username,
      password,
      security: isTLS ? 'tls' : 'none',
      sni: q.get('sni') || '',
    }
  }

  if (scheme === 'http' || scheme === 'https') {
    const isTLS = scheme === 'https' || q.get('tls') === '1' || q.get('security') === 'tls'
    const username = decodeURIComponent(u.username || '')
    const password = decodeURIComponent(u.password || '')
    return {
      protocol: 'http',
      tag,
      address,
      port: u.port ? parseInt(u.port, 10) : (isTLS ? 443 : 80),
      username,
      password,
      security: isTLS ? 'tls' : 'none',
      sni: q.get('sni') || '',
    }
  }

  throw new Error(`不支持的协议 scheme: ${scheme}，当前支持 vless, hysteria2/hy2, socks5, http/https`)
}

export function formatNodeLink(data: ParsedNodeLink): string {
  const tagFrag = data.tag ? `#${encodeURIComponent(data.tag)}` : ''

  if (data.protocol === 'vless') {
    const q = new URLSearchParams()
    const sec = data.security || 'none'
    q.set('security', sec)
    q.set('encryption', data.encryption || 'none')
    if (data.flow) q.set('flow', data.flow)
    if (data.fingerprint) q.set('fp', data.fingerprint)

    if (sec === 'reality') {
      if (data.realityPublicKey) q.set('pbk', data.realityPublicKey)
      if (data.realityShortId) q.set('sid', data.realityShortId)
      if (data.sni) q.set('sni', data.sni)
    } else if (sec === 'tls') {
      if (data.sni) q.set('sni', data.sni)
      if (data.allowInsecure) q.set('allowInsecure', '1')
    }

    const net = data.network || 'tcp'
    q.set('type', net)
    if (net === 'xhttp') {
      if (data.xhttpPath) q.set('path', data.xhttpPath)
      if (data.xhttpHost) q.set('host', data.xhttpHost)
      if (data.xhttpMode) q.set('mode', data.xhttpMode)
    } else if (net === 'ws') {
      if (data.wsPath) q.set('path', data.wsPath)
      if (data.wsHost) q.set('host', data.wsHost)
    } else if (net === 'grpc') {
      if (data.grpcServiceName) q.set('serviceName', data.grpcServiceName)
    }

    return `vless://${data.uuid}@${data.address}:${data.port}?${q.toString()}${tagFrag}`
  }

  if (data.protocol === 'hysteria') {
    const q = new URLSearchParams()
    if (data.sni) q.set('sni', data.sni)
    if (data.allowInsecure) q.set('insecure', '1')
    const qs = q.toString() ? `?${q.toString()}` : ''
    return `hysteria2://${encodeURIComponent(data.password || '')}@${data.address}:${data.port}${qs}${tagFrag}`
  }

  if (data.protocol === 'socks') {
    const isTLS = data.security === 'tls'
    const scheme = isTLS ? 'socks5s' : 'socks5'
    let userPart = ''
    if (data.username) {
      userPart = `${encodeURIComponent(data.username)}:${encodeURIComponent(data.password || '')}@`
    }
    const q = new URLSearchParams()
    if (isTLS && data.sni && data.sni !== data.address) {
      q.set('sni', data.sni)
    }
    const qs = q.toString() ? `?${q.toString()}` : ''
    return `${scheme}://${userPart}${data.address}:${data.port}${qs}${tagFrag}`
  }

  if (data.protocol === 'http') {
    const isTLS = data.security === 'tls'
    const scheme = isTLS ? 'https' : 'http'
    let userPart = ''
    if (data.username) {
      userPart = `${encodeURIComponent(data.username)}:${encodeURIComponent(data.password || '')}@`
    }
    const q = new URLSearchParams()
    if (isTLS && data.sni && data.sni !== data.address) {
      q.set('sni', data.sni)
    }
    const qs = q.toString() ? `?${q.toString()}` : ''
    return `${scheme}://${userPart}${data.address}:${data.port}${qs}${tagFrag}`
  }

  return ''
}
