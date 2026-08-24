export function getTopupHistoryTitleKey(isAdmin: boolean): string {
  return isAdmin ? "Users' top-up history" : 'Top-up history'
}
