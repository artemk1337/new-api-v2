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
export type DocsSection = {
  id: string
  title: string
}

/**
 * H2 anchors rendered by the API-key guide. Keep this list in sync with the
 * explicit ids in content/api-keys.md; the page uses it for both menus.
 */
export const DOCS_SECTIONS: readonly DocsSection[] = [
  { id: 'quick-start', title: 'Быстрый старт' },
  { id: 'initial-setup', title: '1. Первичная настройка' },
  { id: 'claude-code-install', title: '2. Установка Claude Code CLI' },
  { id: 'claude-code', title: '3. Claude Code' },
  { id: 'openai-compatible', title: '4. OpenAI-compatible приложения' },
  { id: 'diagnostics', title: '5. Проверка ключа и диагностика' },
  { id: 'troubleshooting', title: '6. Алгоритм решения проблем' },
  { id: 'error-table', title: '7. Таблица ошибок' },
  { id: 'commands', title: '8. Полезные команды' },
  { id: 'hotkeys', title: '9. Горячие клавиши' },
  { id: 'context', title: '10. Как не потерять контекст' },
  { id: 'faq', title: '11. FAQ' },
  { id: 'support-checklist', title: '12. Чек-лист поддержки' },
  { id: 'compatible-apps', title: '13. Совместимые приложения' },
  { id: 'url-reference', title: '14. Шпаргалка по URL' },
]

export const DOCS_URLS = {
  gateway: 'https://vibecode-api.online',
  openAI: 'https://vibecode-api.online/v1',
  models: 'https://vibecode-api.online/v1/models',
} as const
