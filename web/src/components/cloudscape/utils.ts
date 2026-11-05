export type ClassValue = string | false | null | undefined | ClassValue[]

export function cx(...parts: ClassValue[]): string {
  const out: string[] = []
  const append = (value: ClassValue) => {
    if (Array.isArray(value)) {
      value.forEach(append)
    } else if (value) {
      out.push(value)
    }
  }
  parts.forEach(append)
  return out.join(' ')
}
