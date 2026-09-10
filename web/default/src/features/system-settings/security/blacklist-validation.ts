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
export function normalizeIpBlacklist(value: string): string[] {
  return value
    .split('\n')
    .map((entry) => entry.trim())
    .filter(Boolean)
}

function isValidIpv4(value: string): boolean {
  const parts = value.split('.')
  return (
    parts.length === 4 &&
    parts.every(
      (part) =>
        /^\d{1,3}$/.test(part) &&
        (part.length === 1 || !part.startsWith('0')) &&
        Number(part) <= 255
    )
  )
}

function isValidIpv6(value: string): boolean {
  let address = value

  if (value.includes('.')) {
    const separator = value.lastIndexOf(':')
    if (separator < 0 || !isValidIpv4(value.slice(separator + 1))) return false
    address = `${value.slice(0, separator)}:0:0`
  }

  const compressionCount = address.split('::').length - 1
  if (compressionCount > 1) return false

  const [left, right] = address.split('::')
  const isValidGroup = (group: string) => /^[0-9a-fA-F]{1,4}$/.test(group)
  const leftGroups = left ? left.split(':') : []
  const rightGroups = right ? right.split(':') : []

  if ([...leftGroups, ...rightGroups].some((group) => !isValidGroup(group))) {
    return false
  }

  const groupCount = leftGroups.length + rightGroups.length
  return compressionCount === 1 ? groupCount < 8 : groupCount === 8
}

function parseIp(value: string): 'ipv4' | 'ipv6' | null {
  if (isValidIpv4(value)) return 'ipv4'
  if (value.includes(':') && isValidIpv6(value)) return 'ipv6'
  return null
}

export function isValidIpOrCidr(value: string): boolean {
  const separator = value.indexOf('/')
  if (separator < 0) return parseIp(value) !== null
  if (separator !== value.lastIndexOf('/')) return false

  const address = value.slice(0, separator)
  const prefix = value.slice(separator + 1)
  const version = parseIp(address)
  if (!version || !/^(0|[1-9]\d*)$/.test(prefix)) return false

  const prefixLength = Number(prefix)
  return prefixLength <= (version === 'ipv4' ? 32 : 128)
}
