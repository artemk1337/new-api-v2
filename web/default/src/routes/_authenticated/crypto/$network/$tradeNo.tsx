/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.
*/
import { createFileRoute, Link } from '@tanstack/react-router'

import { DirectCryptoPaymentPage } from '@/features/wallet/components/direct-crypto-payment-page'
import type { DirectUSDTNetwork } from '@/features/wallet/types'

const networks: Record<string, DirectUSDTNetwork> = {
  tron: 'TRON',
  ton: 'TON',
  solana: 'SOLANA',
}

export const Route = createFileRoute('/_authenticated/crypto/$network/$tradeNo')({
  component: CryptoPaymentRoute,
})

function CryptoPaymentRoute() {
  const { network, tradeNo } = Route.useParams()
  const normalized = networks[network.toLowerCase()]
  if (!normalized) {
    return <Link to='/wallet'>Back to wallet</Link>
  }
  return <DirectCryptoPaymentPage network={normalized} tradeNo={tradeNo} />
}
