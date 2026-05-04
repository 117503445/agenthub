import type { ReactNode } from 'react'
import { resolveMarkdownFilesystemHref } from '../lib/filesystem'

type MarkdownBlockType = 'heading' | 'paragraph' | 'list' | 'code'

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

    const heading = /^(#{1,6})\s+(.+)$/.exec(line)
    if (heading) {
      flushParagraph()
      blocks.push({ type: 'heading', level: heading[1].length, text: heading[2].trim() })
      continue
    }

    const unorderedItem = /^\s*[-*]\s+(.+)$/.exec(line)
    if (unorderedItem) {
      flushParagraph()
      const items = [unorderedItem[1].trim()]
      while (index + 1 < lines.length) {
        const nextItem = /^\s*[-*]\s+(.+)$/.exec(lines[index + 1])
        if (!nextItem) {
          break
        }
        items.push(nextItem[1].trim())
        index += 1
      }
      blocks.push({ type: 'list', items, ordered: false })
      continue
    }

    const orderedItem = /^\s*\d+\.\s+(.+)$/.exec(line)
    if (orderedItem) {
      flushParagraph()
      const items = [orderedItem[1].trim()]
      while (index + 1 < lines.length) {
        const nextItem = /^\s*\d+\.\s+(.+)$/.exec(lines[index + 1])
        if (!nextItem) {
          break
        }
        items.push(nextItem[1].trim())
        index += 1
      }
      blocks.push({ type: 'list', items, ordered: true })
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
        <ol key={key}>
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
