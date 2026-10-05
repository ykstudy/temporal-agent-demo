import MarkdownIt from 'markdown-it';
import footnote from 'markdown-it-footnote';
import taskLists from 'markdown-it-task-lists';

/** Display-only Markdown: HTML is escaped and links never invoke a local or executable protocol. */
export function safeMarkdownUrl(value: string): boolean {
  if (/[\u0000-\u0020\u007f]/.test(value) || value.startsWith('//') || value.includes('\\')) return false;
  const scheme = /^([a-z][a-z\d+.-]*):/i.exec(value)?.[1]?.toLowerCase();
  return !scheme || ['http', 'https', 'mailto'].includes(scheme);
}

const markdown = new MarkdownIt({ html: false, linkify: true, typographer: false });
markdown.validateLink = safeMarkdownUrl;
markdown.use(footnote).use(taskLists, { enabled: false });

export function renderMarkdown(content: string): string {
  return markdown.render(content);
}
