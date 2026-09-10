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
import { describe, expect, test } from 'bun:test'
import { isValidIpOrCidr, normalizeIpBlacklist } from './blacklist-validation'

describe('IP blacklist validation', () => {
  test('accepts IP addresses and CIDR ranges', () => {
    expect(isValidIpOrCidr('203.0.113.7')).toBe(true)
    expect(isValidIpOrCidr('10.0.0.0/8')).toBe(true)
    expect(isValidIpOrCidr('2001:db8::1')).toBe(true)
    expect(isValidIpOrCidr('2001:db8::/32')).toBe(true)
    expect(isValidIpOrCidr('::ffff:192.0.2.1')).toBe(true)
    expect(isValidIpOrCidr('::192.0.2.1')).toBe(true)
    expect(isValidIpOrCidr('2001:db8::192.0.2.1')).toBe(true)
    expect(isValidIpOrCidr('::ffff:192.0.2.1/128')).toBe(true)
  })

  test('rejects malformed addresses and prefixes', () => {
    expect(isValidIpOrCidr('203.0.113.999')).toBe(false)
    expect(isValidIpOrCidr('10.0.0.0/33')).toBe(false)
    expect(isValidIpOrCidr('1.2.3.4/032')).toBe(false)
    expect(isValidIpOrCidr('::ffff:192.0.2.1/032')).toBe(false)
    expect(isValidIpOrCidr('2001:db8::/0128')).toBe(false)
    expect(isValidIpOrCidr('2001:db8:::1')).toBe(false)
    expect(isValidIpOrCidr('not-an-ip')).toBe(false)
  })

  test('trims entries and ignores blank lines', () => {
    expect(normalizeIpBlacklist(' 203.0.113.7\n\n10.0.0.0/8 ')).toEqual([
      '203.0.113.7',
      '10.0.0.0/8',
    ])
  })
})
