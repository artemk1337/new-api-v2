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

import {
  guideSections,
  codexSetup,
  continueSetup,
  cursorSetup,
} from './docs-content'
import { DOCS_URLS } from './docs-sections'

export type ApplicationGuide = {
  id: string
  name: string
  platform: string
  download: string
  source: string
  fields: { label: string; value: string }[]
  steps: string[]
  note?: string
  content?: string
}

export function useApplicationGuides(): {
  desktop: ApplicationGuide[]
  mobile: ApplicationGuide[]
} {
  const { t } = useTranslation()
  const apiKey = {
    label: 'API Key',
    value: t('Your key from the API Keys page'),
  }
  const model = {
    label: 'Model ID',
    value: t('Exact model ID from /v1/models'),
  }
  const chatbox: ApplicationGuide = {
    id: 'chatbox',
    name: 'Chatbox AI',
    platform: 'Windows · macOS · Linux',
    download: 'https://chatboxai.app/',
    source: 'https://docs.chatboxai.app/en/guides/providers',
    fields: [
      { label: t('Provider'), value: 'OpenAI API compatible' },
      { label: 'API Host', value: DOCS_URLS.gateway },
      { label: 'API Path', value: '/v1/chat/completions' },
      apiKey,
      model,
    ],
    steps: [
      t(
        'Open Settings → Model Provider → Add and choose OpenAI API compatible.'
      ),
      t('Enter the connection values below, add a model and run Check.'),
      t('Save the provider, start a new chat and select your model.'),
    ],
    note: t(
      'Chatbox joins API Host and API Path. The final address must end with /v1/chat/completions.'
    ),
  }

  return {
    desktop: [
      {
        id: 'claude-code',
        name: 'Claude Code',
        platform: 'Windows · macOS · Linux',
        download: 'https://code.claude.com/docs/en/setup',
        source: 'https://code.claude.com/docs/en/setup',
        fields: [],
        steps: [],
        content: guideSections['claude-code'],
      },
      {
        id: 'codex-cli',
        name: 'ChatGPT / Codex',
        platform: 'Windows · macOS · Linux',
        download: 'https://developers.openai.com/codex/cli/',
        source:
          'https://developers.openai.com/codex/config-file/config-advanced#custom-model-providers',
        fields: [],
        steps: [],
        content: codexSetup,
      },
      {
        id: 'cherry-studio',
        name: 'Cherry Studio',
        platform: 'Windows · macOS · Linux',
        download: 'https://cherryai.com/download/v2',
        source:
          'https://www.cherryai.com/docs/en/pre-basic/settings/providers/',
        fields: [
          { label: t('Provider'), value: 'OpenAI' },
          { label: 'API Host', value: DOCS_URLS.gateway },
          apiKey,
          model,
        ],
        steps: [
          t(
            'Open Settings → Model Provider and add an OpenAI-compatible provider.'
          ),
          t(
            'Enter API Host and API Key, then use Sync models to add your models.'
          ),
          t('Run Model Check, enable the provider and select a model in chat.'),
        ],
        note: t(
          'Cherry Studio adds /v1/chat/completions to API Host automatically.'
        ),
      },
      chatbox,
      {
        id: 'cline',
        name: 'Cline',
        platform: t('VS Code extension'),
        download: 'https://docs.cline.bot/getting-started/installing-cline',
        source: 'https://docs.cline.bot/provider-config/openai-compatible',
        fields: [
          { label: 'API Provider', value: 'OpenAI Compatible' },
          { label: 'Base URL', value: DOCS_URLS.openAI },
          apiKey,
          model,
        ],
        steps: [
          t('Install Cline from VS Code Extensions and open its settings.'),
          t(
            'Choose OpenAI Compatible and enter Base URL, API Key and Model ID.'
          ),
          t(
            'Save settings and send a request. Choose a model with tool calling for agent tasks.'
          ),
        ],
      },
      {
        id: 'continue',
        name: 'Continue.dev',
        platform: t('VS Code extension'),
        download: 'https://docs.continue.dev/getting-started/install',
        source:
          'https://docs.continue.dev/customize/model-providers/top-level/openai',
        fields: [],
        steps: [],
        content: continueSetup,
      },
      {
        id: 'cursor',
        name: 'Cursor',
        platform: 'Windows · macOS · Linux',
        download: 'https://cursor.com/downloads',
        source: 'https://cursor.com/docs/settings/api-keys',
        fields: [{ label: 'Base URL', value: DOCS_URLS.openAI }, apiKey, model],
        steps: [],
        content: cursorSetup,
      },
    ],
    mobile: [
      { ...chatbox, platform: 'iOS · Android' },
      {
        id: 'rikkahub',
        name: 'RikkaHub',
        platform: 'Android',
        download: 'https://rikka-ai.com/download',
        source: 'https://github.com/rikkahub/docs/blob/main/quickstart.mdx',
        fields: [
          { label: t('Provider'), value: 'OpenAI' },
          { label: 'Base URL', value: DOCS_URLS.openAI },
          { label: 'Chat Completions Path', value: '/chat/completions' },
          apiKey,
          model,
        ],
        steps: [
          t('Open Settings → Provider Settings and add an OpenAI provider.'),
          t(
            'Enter Base URL and API Key. Use the Chat Completions path shown below.'
          ),
          t(
            'Open Models, load or add your models, then select one in a new chat.'
          ),
        ],
      },
      {
        id: 'chatterui',
        name: 'ChatterUI',
        platform: 'Android',
        download: 'https://github.com/Vali-98/ChatterUI/releases',
        source: 'https://github.com/Vali-98/ChatterUI',
        fields: [
          { label: t('Connection type'), value: 'Chat Completions' },
          {
            label: 'Completion URL',
            value: `${DOCS_URLS.openAI}/chat/completions`,
          },
          { label: 'Model URL', value: DOCS_URLS.models },
          apiKey,
          model,
        ],
        steps: [
          t(
            'Enable Remote Mode, open Connections → Add Connection and choose Chat Completions.'
          ),
          t('Enter the full Completion URL, Model URL and API Key.'),
          t('Choose a model, save the connection and select it for your chat.'),
        ],
        note: t(
          'Choose Chat Completions to edit the URLs. The built-in OpenAI template uses fixed addresses.'
        ),
      },
    ],
  }
}
