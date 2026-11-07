/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
import apiKeysMarkdown from './content/api-keys.md?raw'

const headings = [
  ...apiKeysMarkdown.matchAll(/<h2 id="([^"]+)"[^>]*>.*?<\/h2>/g),
]
const sections = Object.fromEntries(
  headings.map((heading, index) => [
    heading[1],
    apiKeysMarkdown.slice(heading.index, headings[index + 1]?.index),
  ])
)

export const CLAUDE_SECTION_IDS = [
  'claude-code-install',
  'claude-code',
  'commands',
  'hotkeys',
  'context',
]

export const DESKTOP_SECTION_IDS = [...CLAUDE_SECTION_IDS, 'openai-compatible']

function withoutSectionNumbers(content: string): string {
  return content
    .replaceAll(/(<h[23][^>]*>)\d+\.\s*/g, '$1')
    .replaceAll(/^(#{2,4}\s+)\d+(?:\.\d+)*\.\s*/gm, '$1')
}

const [continueGuide, remainingClients] =
  sections['openai-compatible'].split('### Codex CLI')
const [codexGuide, cursorGuide] = remainingClients.split(
  '### Cline, Cursor и другие клиенты'
)

export const continueSetup = `<h3 id="openai-compatible" class="scroll-mt-24">Continue.dev</h3>\n${continueGuide.slice(continueGuide.indexOf('```yaml'))}`
export const codexSetup = `<h3 id="codex-cli" class="scroll-mt-24">Codex CLI</h3>\n${codexGuide}`
export const cursorSetup = `<h3 id="cursor" class="scroll-mt-24">Cursor</h3>\n${cursorGuide}`

export const guideSections = Object.fromEntries(
  Object.entries(sections).map(([id, content]) => [
    id,
    withoutSectionNumbers(content),
  ])
)

export function withoutGuideHeading(content: string): string {
  return content.replace(/^\s*<h[23][^>]*>.*?<\/h[23]>\s*/, '')
}
