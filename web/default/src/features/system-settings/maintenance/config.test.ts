/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/

import assert from 'node:assert/strict'
import { describe, test } from 'node:test'

import { parseSidebarModulesAdmin } from './config'

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
