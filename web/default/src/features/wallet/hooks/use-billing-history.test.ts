/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import assert from 'node:assert/strict'
import { test } from 'node:test'

import { isCurrentBillingHistoryRequest } from './use-billing-history'

test('billing history ignores a late response from the previous page request', () => {
  const firstPageRequest = 1
  const secondPageRequest = 2

  assert.equal(
    isCurrentBillingHistoryRequest(firstPageRequest, secondPageRequest),
    false
  )
  assert.equal(
    isCurrentBillingHistoryRequest(secondPageRequest, secondPageRequest),
    true
  )
})
