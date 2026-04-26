import { type ClassValue, clsx } from 'clsx'
import { twMerge } from 'tailwind-merge'

// cn 合并 inputs 参数中的 Tailwind 类名。
export function cn(...inputs: ClassValue[]) {
  return twMerge(clsx(inputs))
}
