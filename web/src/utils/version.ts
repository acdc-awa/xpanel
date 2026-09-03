/**
 * 语义化版本比对工具
 */

export function normVersion(s: string): string {
  s = (s || '').trim().replace(/^v/i, '')
  const dashIdx = s.indexOf('-')
  if (dashIdx >= 0) {
    s = s.slice(0, dashIdx)
  }
  return s || '0'
}

/**
 * 语义化版本比较：
 * - 返回 -1：a < b
 * - 返回 0：a === b
 * - 返回 1：a > b
 */
export function compareVersion(a: string, b: string): number {
  const na = normVersion(a)
    .split('.')
    .map((n) => parseInt(n, 10) || 0)
  const nb = normVersion(b)
    .split('.')
    .map((n) => parseInt(n, 10) || 0)

  const len = Math.max(na.length, nb.length)
  for (let i = 0; i < len; i++) {
    const ai = na[i] ?? 0
    const bi = nb[i] ?? 0
    if (ai < bi) return -1
    if (ai > bi) return 1
  }
  return 0
}
