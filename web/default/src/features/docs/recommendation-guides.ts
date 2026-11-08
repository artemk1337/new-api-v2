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
import { useTranslation } from 'react-i18next'

export type RecommendationGuide = {
  id: string
  title: string
  content: string
  group?: 'indexers'
}

const skillExample = `---
name: review-change
description: Review a code change before committing.
---
Read the diff. Find bugs and missing tests.
Report each issue with its file and line.
`

export function useRecommendationGuides(): RecommendationGuide[] {
  const { t } = useTranslation()
  return [
    {
      id: 'recommendations-skills',
      title: t('Skills'),
      content: [
        t(
          'A skill is a saved set of instructions for a recurring task. Use one when you keep explaining the same review or testing steps.'
        ),
        `1. ${t(
          'In Claude Code, create .claude/skills/review-change/SKILL.md in your project and paste this example.'
        )}`,
        `   \`\`\`markdown\n${skillExample
          .split('\n')
          .map((line) => `   ${line}`)
          .join('\n')}   \`\`\``,
        `2. ${t(
          'Run /review-change in the chat. Store personal skills in ~/.claude/skills/ to use them across projects.'
        )}`,
        '[Claude Code Skills](https://code.claude.com/docs/en/skills)',
      ].join('\n\n'),
    },
    {
      id: 'recommendations-indexers',
      title: t('Indexer overview'),
      group: 'indexers',
      content: [
        t(
          'A code indexer builds a map of project files, functions and their relationships. The assistant uses this map to find the code needed for a task.'
        ),
        t(
          'It helps trace function calls, find related code and assess which parts a change may affect. Focused searches can reduce repeated file reading and token use.'
        ),
        t(
          'An index is useful when a task spans several files or you are exploring an unfamiliar project. Check findings against the source code and keep the index current.'
        ),
      ].join('\n\n'),
    },
    {
      id: 'recommendations-serena',
      title: 'Serena',
      group: 'indexers',
      content: [
        t(
          'A code indexer helps the assistant find functions and their callers in a large project. Serena provides code navigation tools through MCP, the protocol for connecting tools to an assistant.'
        ),
        `1. ${t(
          'Install Serena using its documentation, open a terminal in your project folder and connect it to Claude Code.'
        )}`,
        '   ```shell\n   serena setup claude-code\n   ```',
        `2. ${t(
          'Restart the Claude Code session and check the connection with /mcp. Then give it a concrete task.'
        )}`,
        `   ${t(
          'Find the login handler with Serena. Show its callers and the files I need to change to add an authorization check.'
        )}`,
        '[Serena](https://oraios.github.io/serena/02-usage/010_installation.html) · [Claude Code MCP](https://oraios.github.io/serena/02-usage/030_clients.html#claude-code)',
      ].join('\n\n'),
    },
    {
      id: 'recommendations-cbm',
      title: 'Codebase Memory (CBM)',
      group: 'indexers',
      content: [
        t(
          'CBM stores a local index of code and its relationships. Use it to find symbols and trace function calls.'
        ),
        `1. ${t('Install CBM using the project instructions and select the Claude Code integration.')}`,
        '   [Codebase Memory MCP](https://github.com/DeusData/codebase-memory-mcp#installation)',
        `2. ${t('Restart Claude Code, check CBM with /mcp and ask it to index your project.')}`,
        t(
          'Example task: Use CBM to find the payment handler, trace its callers and show the source files.'
        ),
        t(
          'Start with one tool and check that it supports your project language.'
        ),
      ].join('\n\n'),
    },
  ]
}
