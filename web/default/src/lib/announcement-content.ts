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
/** Separate an explicit leading Markdown heading from the collapsible body. */
export function splitAnnouncementContent(
  content: string,
  title?: string
): {
  title: string
  body: string
} {
  if (title?.trim()) return { title: title.trim(), body: content }
  const match = content
    .trimStart()
    .match(/^(?:#{1,6} +(.+?)|\*\*(.+?)\*\*)[ \t]*(?:\r?\n|$)/)
  if (!match) return { title: '', body: content }
  return {
    title: match[1] || match[2],
    body: content.trimStart().slice(match[0].length).trimStart(),
  }
}
