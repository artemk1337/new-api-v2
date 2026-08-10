/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published by
the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export type ApiKeySnippetLanguage = 'curl' | 'python' | 'node' | 'go'

type TokenKeyResponse = {
  success: boolean
  data?: { key: string }
}

export type TokenKeyFetcher = (id: number) => Promise<TokenKeyResponse>

export function maskApiKey(key: string): string {
  if (key.length <= 8) return '*'.repeat(key.length)
  const hiddenLength = Math.max(8, key.length - 9)
  return `${key.slice(0, 5)}${'*'.repeat(hiddenLength)}${key.slice(-4)}`
}

export function normalizeApiKey(key: string): string {
  return key.startsWith('sk-') ? key : `sk-${key}`
}

export async function resolveTokenKey(
  id: number,
  fetcher: TokenKeyFetcher
): Promise<string | null> {
  try {
    const result = await fetcher(id)
    if (!result.success || !result.data?.key) return null
    return normalizeApiKey(result.data.key)
  } catch {
    return null
  }
}

export function normalizeServerAddress(address: string): string {
  const withoutTrailingSlash = address.replace(/\/+$/, '')
  return withoutTrailingSlash.replace(/\/v1$/i, '')
}

export function resolveServerAddress(
  status: unknown,
  fallback: string
): string {
  const statusRecord = status as Record<string, unknown> | null
  const dataRecord = statusRecord?.data as Record<string, unknown> | undefined
  const candidate =
    statusRecord?.server_address ??
    statusRecord?.serverAddress ??
    dataRecord?.server_address ??
    dataRecord?.serverAddress

  return typeof candidate === 'string' && candidate
    ? normalizeServerAddress(candidate)
    : normalizeServerAddress(fallback)
}

export function buildApiKeySnippets(
  apiKey: string,
  baseUrl: string
): Record<ApiKeySnippetLanguage, string> {
  const endpoint = `${baseUrl}/chat/completions`
  const requestBody = JSON.stringify(
    {
      model: 'gpt-4o-mini',
      messages: [{ role: 'user', content: 'Hello!' }],
    },
    null,
    2
  )

  return {
    curl: [
      `curl ${endpoint} \\`,
      `  -H "Authorization: Bearer ${apiKey}" \\`,
      '  -H "Content-Type: application/json" \\',
      `  -d '${requestBody.replaceAll('\n', '\n     ')}'`,
    ].join('\n'),
    python: [
      'from openai import OpenAI',
      '',
      `client = OpenAI(api_key="${apiKey}", base_url="${baseUrl}")`,
      '',
      'response = client.chat.completions.create(',
      '    model="gpt-4o-mini",',
      '    messages=[{"role": "user", "content": "Hello!"}],',
      ')',
      'print(response.choices[0].message.content)',
    ].join('\n'),
    node: [
      "import OpenAI from 'openai';",
      '',
      'const client = new OpenAI({',
      `  apiKey: '${apiKey}',`,
      `  baseURL: '${baseUrl}',`,
      '});',
      '',
      'const response = await client.chat.completions.create({',
      "  model: 'gpt-4o-mini',",
      "  messages: [{ role: 'user', content: 'Hello!' }],",
      '});',
      'console.log(response.choices[0].message.content);',
    ].join('\n'),
    go: [
      'package main',
      '',
      'import (',
      '  "bytes"',
      '  "net/http"',
      ')',
      '',
      'func main() {',
      `  body := bytes.NewBufferString(\`${requestBody}\`)`,
      `  req, _ := http.NewRequest("POST", "${endpoint}", body)`,
      `  req.Header.Set("Authorization", "Bearer ${apiKey}")`,
      '  req.Header.Set("Content-Type", "application/json")',
      '  _, _ = http.DefaultClient.Do(req)',
      '}',
    ].join('\n'),
  }
}
