export function formatGb(gb: number): string {
  if (gb >= 1024) return `${(gb / 1024).toFixed(2)} TB`
  return `${gb.toFixed(1)} GB`
}

export function formatMoney(n: number): string {
  return `¥ ${n.toFixed(2)}`
}

export function formatTraffic(gb: number): string {
  if (gb >= 1024) return `${(gb / 1024).toFixed(2)} TB`
  return `${gb.toFixed(1)} GB`
}
