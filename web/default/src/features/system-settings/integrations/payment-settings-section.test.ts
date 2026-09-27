// @ts-nocheck -- the workspace does not expose Bun test types to tsgo yet.
import { describe, expect, it } from 'bun:test'

import {
  normalizeUSDTReceivingWalletsValue,
  usdtReceivingWalletsSchema,
} from './payment-settings-section'

describe('USDT receiving wallet settings', () => {
  it('normalizes legacy lowercase network names', () => {
    const rawTonAddress =
      '0:B113A994B5024A16719F69139328EB759596C38A25F59028B146FECDC3621DFE'
    const result = usdtReceivingWalletsSchema.safeParse([
      { network: 'ton', address: rawTonAddress },
    ])

    expect(result.success).toBe(true)
    if (result.success) {
      expect(result.data[0].network).toBe('TON')
    }
  })

  it('rejects malformed addresses and incomplete Solana wallets', () => {
    expect(
      usdtReceivingWalletsSchema.safeParse([
        { network: 'TON', address: 'EQCexample' },
      ]).success
    ).toBe(false)
    expect(
      usdtReceivingWalletsSchema.safeParse([
        {
          network: 'SOLANA',
          address: '11111111111111111111111111111111',
          owner: '11111111111111111111111111111111',
        },
      ]).success
    ).toBe(false)
    expect(
      usdtReceivingWalletsSchema.safeParse([
        {
          network: 'SOLANA',
          address: '11111111111111111111111111111111',
          destination: '11111111111111111111111111111111',
        },
      ]).success
    ).toBe(true)
  })

  it('allows an invalid placeholder while a wallet is disabled', () => {
    expect(
      usdtReceivingWalletsSchema.safeParse([
        {
          network: 'SOLANA',
          address: 'placeholder',
          enabled: false,
        },
      ]).success
    ).toBe(true)
  })

  it('treats an empty wallet array as the legacy single-wallet mode', () => {
    expect(normalizeUSDTReceivingWalletsValue(' [ ] ')).toBe('')
    expect(
      normalizeUSDTReceivingWalletsValue(
        '[{"network":"TRON","address":"Texample"}]'
      )
    ).toContain('TRON')
  })
})
