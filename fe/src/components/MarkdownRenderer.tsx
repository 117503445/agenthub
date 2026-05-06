import type { ReactNode } from 'react'
import { resolveMarkdownFilesystemHref } from '../lib/filesystem'

type MarkdownBlockType = 'heading' | 'paragraph' | 'list' | 'code' | 'table'

interface MarkdownBlock {
  /** type 表示块类型。 */
  type: MarkdownBlockType
  /** level 表示标题层级。 */
  level?: number
  /** text 表示段落、标题或代码内容。 */
  text?: string
  /** items 表示列表条目。 */
  items?: string[]
  /** ordered 表示是否为有序列表。 */
  ordered?: boolean
  /** start 表示有序列表的起始编号。 */
  start?: number
  /** headers 表示表格标题行。 */
  headers?: string[]
  /** rows 表示表格内容行。 */
  rows?: string[][]
}

interface MarkdownListMarker {
  /** ordered 表示当前列表项是否为有序列表。 */
  ordered: boolean
  /** indent 表示列表项前导空白数量。 */
  indent: number
  /** text 表示列表项正文。 */
  text: string
  /** number 表示有序列表项的显式编号。 */
  number?: number
}

interface MarkdownRendererProps {
  /** text 表示需要渲染的 markdown 文本。 */
  text: string
  /** projectRoot 表示相对文件路径所属的 project 根目录。 */
  projectRoot?: string
}

// MarkdownRenderer 使用 text 和 projectRoot 参数渲染安全的轻量 markdown 内容。
export function MarkdownRenderer({ text, projectRoot }: MarkdownRendererProps) {
  const blocks = splitMarkdownBlocks(text)
  if (blocks.length === 0) {
    return null
  }
  return (
    <div data-testid="assistant-markdown" className="markdown-body">
      {blocks.map((block, index) => renderMarkdownBlock(block, `block-${index}`, projectRoot))}
    </div>
  )
}

// splitMarkdownBlocks 使用 text 参数拆分 markdown 块。
function splitMarkdownBlocks(text: string) {
  const lines = text.replace(/\r\n/g, '\n').split('\n')
  const blocks: MarkdownBlock[] = []
  let paragraph: string[] = []
  let code: string[] | null = null

  // flushParagraph 将当前段落写入 blocks。
  const flushParagraph = () => {
    const content = paragraph.join('\n').trim()
    if (content) {
      blocks.push({ type: 'paragraph', text: content })
    }
    paragraph = []
  }

  for (let index = 0; index < lines.length; index += 1) {
    const line = lines[index]
    if (code) {
      if (/^```/.test(line.trim())) {
        blocks.push({ type: 'code', text: code.join('\n') })
        code = null
      } else {
        code.push(line)
      }
      continue
    }

    if (/^```/.test(line.trim())) {
      flushParagraph()
      code = []
      continue
    }

    if (line.trim() === '') {
      flushParagraph()
      continue
    }

    if (isMarkdownTableStart(lines, index)) {
      flushParagraph()
      const parsedTable = parseMarkdownTable(lines, index)
      blocks.push(parsedTable.block)
      index = parsedTable.nextIndex - 1
      continue
    }

    const heading = /^(#{1,6})\s+(.+)$/.exec(line)
    if (heading) {
      flushParagraph()
      blocks.push({ type: 'heading', level: heading[1].length, text: heading[2].trim() })
      continue
    }

    const listMarker = matchMarkdownListMarker(line)
    if (listMarker) {
      flushParagraph()
      const parsedList = parseMarkdownList(lines, index, listMarker)
      blocks.push(parsedList.block)
      index = parsedList.nextIndex - 1
      continue
    }

    paragraph.push(line)
  }

  if (code) {
    blocks.push({ type: 'code', text: code.join('\n') })
  }
  flushParagraph()
  return blocks
}

// matchMarkdownListMarker 使用 line 参数匹配 markdown 列表项标记。
function matchMarkdownListMarker(line: string): MarkdownListMarker | null {
  const orderedItem = /^(\s*)(\d+)\.\s+(.+)$/.exec(line)
  if (orderedItem) {
    return {
      ordered: true,
      indent: orderedItem[1].length,
      text: orderedItem[3].trim(),
      number: Number.parseInt(orderedItem[2], 10),
    }
  }
  const unorderedItem = /^(\s*)[-*]\s+(.+)$/.exec(line)
  if (unorderedItem) {
    return {
      ordered: false,
      indent: unorderedItem[1].length,
      text: unorderedItem[2].trim(),
    }
  }
  return null
}

// parseMarkdownList 使用 lines、startIndex 和 firstMarker 参数解析连续 markdown 列表。
function parseMarkdownList(lines: string[], startIndex: number, firstMarker: MarkdownListMarker) {
  const items: string[] = []
  const current: string[] = []
  let index = startIndex

  // flushItem 将当前列表项写入 items。
  const flushItem = () => {
    const text = current.join('\n').trim()
    if (text) {
      items.push(text)
    }
    current.length = 0
  }

  while (index < lines.length) {
    const line = lines[index]
    const marker = matchMarkdownListMarker(line)
    if (marker && isSameListMarker(marker, firstMarker)) {
      flushItem()
      current.push(marker.text)
      index += 1
      continue
    }
    if (line.trim() === '') {
      const nextIndex = nextNonEmptyLineIndex(lines, index + 1)
      if (nextIndex >= 0 && isSameListMarker(matchMarkdownListMarker(lines[nextIndex]), firstMarker)) {
        index = nextIndex
        continue
      }
      break
    }
    if (marker || isMarkdownBlockBoundary(line)) {
      break
    }
    if (current.length === 0) {
      break
    }
    current.push(line.trim())
    index += 1
  }

  flushItem()
  return {
    block: {
      type: 'list' as const,
      items,
      ordered: firstMarker.ordered,
      start: firstMarker.number,
    },
    nextIndex: index,
  }
}

// isSameListMarker 使用 marker 和 firstMarker 参数判断列表项是否属于同一列表。
function isSameListMarker(marker: MarkdownListMarker | null, firstMarker: MarkdownListMarker) {
  return Boolean(marker && marker.ordered === firstMarker.ordered && marker.indent === firstMarker.indent)
}

// nextNonEmptyLineIndex 使用 lines 和 startIndex 参数查找下一条非空行。
function nextNonEmptyLineIndex(lines: string[], startIndex: number) {
  for (let index = startIndex; index < lines.length; index += 1) {
    if (lines[index].trim() !== '') {
      return index
    }
  }
  return -1
}

// isMarkdownBlockBoundary 使用 line 参数判断当前行是否应结束列表块。
function isMarkdownBlockBoundary(line: string) {
  const trimmed = line.trim()
  return /^```/.test(trimmed) || /^(#{1,6})\s+(.+)$/.test(line)
}

// isMarkdownTableStart 使用 lines 和 index 参数判断当前位置是否是 markdown 表格起点。
function isMarkdownTableStart(lines: string[], index: number) {
  return index + 1 < lines.length && isMarkdownTableRow(lines[index]) && isMarkdownTableSeparator(lines[index + 1])
}

// parseMarkdownTable 使用 lines 和 startIndex 参数解析 markdown 表格。
function parseMarkdownTable(lines: string[], startIndex: number) {
  const headers = splitMarkdownTableCells(lines[startIndex])
  const rows: string[][] = []
  let index = startIndex + 2
  while (index < lines.length && isMarkdownTableRow(lines[index])) {
    rows.push(splitMarkdownTableCells(lines[index]))
    index += 1
  }
  return {
    block: {
      type: 'table' as const,
      headers,
      rows,
    },
    nextIndex: index,
  }
}

// isMarkdownTableRow 使用 line 参数判断是否是 markdown 表格行。
function isMarkdownTableRow(line: string) {
  const trimmed = line.trim()
  return trimmed.startsWith('|') && trimmed.endsWith('|') && splitMarkdownTableCells(trimmed).length > 1
}

// isMarkdownTableSeparator 使用 line 参数判断是否是 markdown 表格分隔行。
function isMarkdownTableSeparator(line: string) {
  if (!isMarkdownTableRow(line)) {
    return false
  }
  return splitMarkdownTableCells(line).every((cell) => /^:?-{3,}:?$/.test(cell.trim()))
}

// splitMarkdownTableCells 使用 line 参数拆分 markdown 表格单元格。
function splitMarkdownTableCells(line: string) {
  const trimmed = line.trim().replace(/^\|/, '').replace(/\|$/, '')
  const cells: string[] = []
  let current = ''
  let escaped = false
  for (const char of trimmed) {
    if (escaped) {
      current += char
      escaped = false
      continue
    }
    if (char === '\\') {
      escaped = true
      continue
    }
    if (char === '|') {
      cells.push(current.trim())
      current = ''
      continue
    }
    current += char
  }
  if (escaped) {
    current += '\\'
  }
  cells.push(current.trim())
  return cells
}

// renderMarkdownBlock 使用 block、key 和 projectRoot 参数渲染 markdown 块。
function renderMarkdownBlock(block: MarkdownBlock, key: string, projectRoot?: string) {
  if (block.type === 'heading') {
    const level = Math.min(Math.max(block.level ?? 2, 1), 6)
    if (level === 1) {
      return <h1 key={key}>{renderInlineMarkdown(block.text ?? '', key, projectRoot)}</h1>
    }
    if (level === 2) {
      return <h2 key={key}>{renderInlineMarkdown(block.text ?? '', key, projectRoot)}</h2>
    }
    if (level === 3) {
      return <h3 key={key}>{renderInlineMarkdown(block.text ?? '', key, projectRoot)}</h3>
    }
    if (level === 4) {
      return <h4 key={key}>{renderInlineMarkdown(block.text ?? '', key, projectRoot)}</h4>
    }
    if (level === 5) {
      return <h5 key={key}>{renderInlineMarkdown(block.text ?? '', key, projectRoot)}</h5>
    }
    return <h6 key={key}>{renderInlineMarkdown(block.text ?? '', key, projectRoot)}</h6>
  }
  if (block.type === 'list') {
    if (block.ordered) {
      return (
        <ol key={key} start={block.start}>
          {(block.items ?? []).map((item, index) => (
            <li key={`${key}-item-${index}`}>{renderInlineMarkdown(item, `${key}-item-${index}`, projectRoot)}</li>
          ))}
        </ol>
      )
    }
    return (
      <ul key={key}>
        {(block.items ?? []).map((item, index) => (
          <li key={`${key}-item-${index}`}>{renderInlineMarkdown(item, `${key}-item-${index}`, projectRoot)}</li>
        ))}
      </ul>
    )
  }
  if (block.type === 'code') {
    return (
      <pre key={key}>
        <code>{block.text}</code>
      </pre>
    )
  }
  if (block.type === 'table') {
    return (
      <div key={key} className="markdown-table-wrap">
        <table>
          <thead>
            <tr>
              {(block.headers ?? []).map((header, index) => (
                <th key={`${key}-head-${index}`}>{renderInlineMarkdown(header, `${key}-head-${index}`, projectRoot)}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {(block.rows ?? []).map((row, rowIndex) => (
              <tr key={`${key}-row-${rowIndex}`}>
                {(block.headers ?? row).map((_, cellIndex) => (
                  <td key={`${key}-row-${rowIndex}-cell-${cellIndex}`}>
                    {renderInlineMarkdown(row[cellIndex] ?? '', `${key}-row-${rowIndex}-cell-${cellIndex}`, projectRoot)}
                  </td>
                ))}
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    )
  }
  return <p key={key}>{renderInlineMarkdown(block.text ?? '', key, projectRoot)}</p>
}

// renderInlineMarkdown 使用 text、keyPrefix 和 projectRoot 参数渲染行内 markdown。
function renderInlineMarkdown(text: string, keyPrefix: string, projectRoot?: string) {
  const nodes: ReactNode[] = []
  const pattern = /(\*\*[^*]+\*\*|`[^`]+`|\[[^\]]+\]\([^)]+\))/g
  let cursor = 0
  let match: RegExpExecArray | null
  while ((match = pattern.exec(text)) !== null) {
    if (match.index > cursor) {
      nodes.push(text.slice(cursor, match.index))
    }
    nodes.push(renderInlineToken(match[0], `${keyPrefix}-inline-${nodes.length}`, projectRoot))
    cursor = match.index + match[0].length
  }
  if (cursor < text.length) {
    nodes.push(text.slice(cursor))
  }
  return nodes
}

// renderInlineToken 使用 token、key 和 projectRoot 参数渲染单个行内 token。
function renderInlineToken(token: string, key: string, projectRoot?: string) {
  if (token.startsWith('**') && token.endsWith('**')) {
    return <strong key={key}>{token.slice(2, -2)}</strong>
  }
  if (token.startsWith('`') && token.endsWith('`')) {
    return <code key={key}>{token.slice(1, -1)}</code>
  }
  const link = /^\[([^\]]+)\]\(([^)]+)\)$/.exec(token)
  if (link) {
    const href = sanitizeMarkdownHref(link[2], projectRoot)
    if (href) {
      return (
        <a key={key} href={href} target="_blank" rel="noreferrer">
          {link[1]}
        </a>
      )
    }
  }
  return token
}

// sanitizeMarkdownHref 使用 href 和 projectRoot 参数限制 markdown 链接地址。
function sanitizeMarkdownHref(href: string, projectRoot?: string) {
  const resolved = resolveMarkdownFilesystemHref(href, projectRoot)
  if (/^(https?:\/\/|\/|#)/i.test(resolved)) {
    return resolved
  }
  return ''
}
