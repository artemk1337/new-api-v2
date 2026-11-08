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

import { RichContent } from '@/components/rich-content'

import { DOCS_URLS } from './docs-sections'

const settings = JSON.stringify(
  {
    'claudeCode.disableLoginPrompt': true,
    'claudeCode.environmentVariables': [
      { name: 'ANTHROPIC_BASE_URL', value: DOCS_URLS.gateway },
      { name: 'ANTHROPIC_AUTH_TOKEN', value: 'sk-xxxxxxxx' },
      { name: 'ANTHROPIC_MODEL', value: 'MODEL_ID' },
    ],
  },
  null,
  2
)

export function VscodeGuide() {
  const { t } = useTranslation()
  const content = [
    `1. ${t(
      'Install the official Claude Code extension by Anthropic in VS Code. It includes everything needed for the chat panel.'
    )}`,
    '   [Claude Code — Visual Studio Marketplace](https://marketplace.visualstudio.com/items?itemName=anthropic.claude-code)',
    `2. ${t(
      'Open the Command Palette with Cmd+Shift+P on macOS or Ctrl+Shift+P on Windows/Linux. Run Preferences: Open User Settings (JSON).'
    )}`,
    `3. ${t(
      'Add these fields to your existing settings. Replace sk-xxxxxxxx with your VibeCode API key and MODEL_ID with the exact Claude model ID from the model catalog.'
    )}`,
    `   \`\`\`json\n${settings
      .split('\n')
      .map((line) => `   ${line}`)
      .join('\n')}\n   \`\`\``,
    `   ${t(
      'Use https://vibecode-api.online without /v1. Save the key in User Settings, outside your project repository.'
    )}`,
    `4. ${t(
      'Run Developer: Reload Window, then open Claude Code from the Command Palette. Drag the chat panel to the right side of the editor if needed.'
    )}`,
    `5. ${t(
      'Send a short test message and check that the request appears in your VibeCode usage logs. If the login screen still appears, check claudeCode.disableLoginPrompt and reload the window.'
    )}`,
    t(
      'If Claude Code is already installed, skip reinstalling it. See the [installation guide](#claude-code-install) and [connection settings](#claude-code). For the VS Code panel, install the extension and apply the settings above.'
    ),
    '[Claude Code — VS Code](https://code.claude.com/docs/en/vs-code) · [LLM gateway](https://code.claude.com/docs/en/llm-gateway-connect)',
  ].join('\n\n')

  return (
    <RichContent
      mode='markdown'
      content={content}
      className='prose-neutral dark:prose-invert max-w-none'
    />
  )
}
