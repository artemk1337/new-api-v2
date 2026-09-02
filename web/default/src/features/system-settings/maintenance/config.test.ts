/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import {
  HEADER_NAV_DEFAULT,
  parseHeaderNavModules,
  parseSidebarModulesAdmin,
  serializeHeaderNavModules,
} from './config'

describe('header navigation config', () => {
  test('enables the referral program page by default', () => {
    assert.equal(parseHeaderNavModules('').referralProgram, true)
  })

  test('parses and serializes the referral program visibility', () => {
    const config = parseHeaderNavModules(
      JSON.stringify({ referralProgram: false })
    )

    assert.equal(config.referralProgram, false)
    assert.equal(
      JSON.parse(serializeHeaderNavModules(config)).referralProgram,
      false
    )
    assert.equal(HEADER_NAV_DEFAULT.referralProgram, true)
  })
})

describe('sidebar modules admin config', () => {
  test('ignores legacy chat section from persisted settings', () => {
    const config = parseSidebarModulesAdmin(
      JSON.stringify({
        chat: { enabled: true, playground: true, chat: true },
        console: { enabled: true, detail: false },
      })
    )

    assert.equal(config.chat, undefined)
    assert.equal(config.console.detail, false)
  })
})
