export function getBillingSectionRedirect(section: string): string | null {
  if (section === 'currency' || section === 'currency-exchange-rate') {
    return 'platform-currencies'
  }
  return null
}
