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
export function buildSpeechSample(
  lang: string,
  ctx: {
    baseUrl: string
    endpointPath: string
    apiKeyEnv: string
    modelName: string
  }
): string {
  const url = `${ctx.baseUrl}${ctx.endpointPath}`
  const input = 'Hello! This is a text-to-speech example.'
  const body = JSON.stringify(
    { model: ctx.modelName, input, voice: 'alloy', response_format: 'mp3' },
    null,
    2
  )

  if (lang === 'curl') {
    return [
      `curl ${url} \\`,
      `  -H "Authorization: Bearer $${ctx.apiKeyEnv}" \\`,
      `  -H "Content-Type: application/json" \\`,
      `  -d '${body.replaceAll('\n', '\n     ')}' \\`,
      '  --output speech.mp3',
    ].join('\n')
  }

  if (lang === 'python') {
    return [
      'import os',
      'from openai import OpenAI',
      '',
      'client = OpenAI(',
      `    base_url="${ctx.baseUrl}/v1",`,
      `    api_key=os.environ["${ctx.apiKeyEnv}"],`,
      ')',
      '',
      'with client.audio.speech.with_streaming_response.create(',
      `    model="${ctx.modelName}",`,
      `    input="${input}",`,
      '    voice="alloy",',
      '    response_format="mp3",',
      ') as response:',
      '    response.stream_to_file("speech.mp3")',
    ].join('\n')
  }

  if (lang === 'typescript') {
    return [
      "import OpenAI from 'openai'",
      "import { writeFile } from 'node:fs/promises'",
      '',
      'const client = new OpenAI({',
      `  baseURL: '${ctx.baseUrl}/v1',`,
      `  apiKey: process.env.${ctx.apiKeyEnv},`,
      '})',
      '',
      'const response = await client.audio.speech.create({',
      `  model: '${ctx.modelName}',`,
      `  input: '${input}',`,
      "  voice: 'alloy',",
      "  response_format: 'mp3',",
      '})',
      '',
      "await writeFile('speech.mp3', Buffer.from(await response.arrayBuffer()))",
    ].join('\n')
  }

  return [
    "import { writeFile } from 'node:fs/promises'",
    '',
    `const response = await fetch('${url}', {`,
    "  method: 'POST',",
    '  headers: {',
    `    Authorization: \`Bearer \${process.env.${ctx.apiKeyEnv}}\`,`,
    "    'Content-Type': 'application/json',",
    '  },',
    `  body: JSON.stringify(${body.replaceAll('\n', '\n  ')}),`,
    '})',
    '',
    'if (!response.ok) throw new Error(await response.text())',
    "await writeFile('speech.mp3', Buffer.from(await response.arrayBuffer()))",
  ].join('\n')
}
