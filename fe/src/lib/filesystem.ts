import { readStoredAgentHubToken } from './auth'

// getFilesystemContentUrl 使用 filePath 参数生成文件系统内容接口地址。
export function getFilesystemContentUrl(filePath: string) {
  const pageUrl = window.location.href.split('#')[0]
  const url = new URL('fs/content', new URL('.', pageUrl))
  url.searchParams.set('path', filePath)
  const token = readStoredAgentHubToken()
  if (token) {
    url.searchParams.set('token', token)
  }
  return url.toString()
}

// resolveMarkdownFilesystemHref 使用 href 和 projectRoot 参数解析 markdown 文件路径链接。
export function resolveMarkdownFilesystemHref(href: string, projectRoot?: string) {
  const trimmed = href.trim()
  if (!trimmed || /^(https?:\/\/|#)/i.test(trimmed)) {
    return trimmed
  }
  const filesystemPath = stripFilesystemLineSuffix(trimmed)
  if (isFilesystemAbsolutePath(filesystemPath)) {
    return getFilesystemContentUrl(filesystemPath)
  }
  const root = projectRoot?.trim()
  if (!root || isUnsafeRelativeFilesystemPath(filesystemPath)) {
    return ''
  }
  return getFilesystemContentUrl(joinProjectPath(root, filesystemPath))
}

// stripFilesystemLineSuffix 使用 value 参数去掉文件路径末尾的行号或行列号后缀。
function stripFilesystemLineSuffix(value: string) {
  return value.trim().replace(/:\d+(?::\d+)?$/, '')
}

// isFilesystemAbsolutePath 使用 value 参数判断是否为后端文件系统绝对路径。
function isFilesystemAbsolutePath(value: string) {
  return (value.startsWith('/') && !value.startsWith('//')) || /^[a-zA-Z]:[\\/]/.test(value) || value.startsWith('\\\\')
}

// isUnsafeRelativeFilesystemPath 使用 value 参数判断是否为不应转换的相对路径。
function isUnsafeRelativeFilesystemPath(value: string) {
  return /^[a-zA-Z][a-zA-Z\d+.-]*:/.test(value) || value.startsWith('//')
}

// joinProjectPath 使用 projectRoot 和 relativePath 参数拼接 project 根目录和相对文件路径。
function joinProjectPath(projectRoot: string, relativePath: string) {
  const root = projectRoot.trim()
  const relative = relativePath.trim().replace(/^[.][\\/]/, '')
  if (/^[a-zA-Z]:[\\/]/.test(root) || root.includes('\\')) {
    return `${root.replace(/[\\/]+$/, '')}\\${relative.replace(/[\\/]+/g, '\\')}`
  }
  return `${root.replace(/\/+$/, '')}/${relative.replace(/\\/g, '/')}`
}
