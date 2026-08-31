---
name: direct-crypto-payments
description: >-
  Реализовывать и ревьюить прямые crypto-платежи с переводом на receiving
  wallet, включая точную сумму, blockchain watcher, settlement и reconciliation.
  Применять только к direct crypto flow, а не к hosted-провайдерам.
---

# Безопасные direct crypto-платежи

Главный инвариант — не зачислить деньги без проверяемого on-chain факта и не
зачислить один перевод дважды.

- Конфигурация должна fail closed: принимать платёж можно только если включён
  метод, задан валидный receiving address и явно поддерживаемая пара
  currency/network. `PayMethods` из БД — authoritative gate для всех create,
  status и callback/reconciliation endpoints; одного флага провайдера
  недостаточно. UI может сделать сеть доступной только при наличии полного
  watcher+contract+settlement+reconciliation пути. Сейчас поддержаны только
  TRON/TRC20, TON и Solana; любую будущую сеть показывай
  disabled/невыбираемой и никогда не принимай до полного end-to-end пути.
- Receiving address хранится отдельным параметром для каждой сети (например,
  `USDTTRC20ReceivingAddress`, `USDTTONReceivingAddress`,
  `USDTSolanaReceivingAddress`). Для Solana дополнительно храни точный
  `USDTSolanaReceivingTokenAccount`: платёжный адрес invoice — именно SPL
  token account, а owner address используется только для проверки владения.
  Для поддержанной сети в invoice сохраняй immutable snapshot адреса; ротация
  настройки не меняет уже созданные заказы.
- Currency/network/contract и receiving address фиксированы серверным
  контрактом; currency direct flow всегда `USDT`. Не принимай их как
  доверенные поля клиента и не меняй ими immutable snapshot уже созданного
  invoice/order.
- Суммы храни и сравнивай в точных целых минимальных единицах токена, без
  `float`; случайный хвост и минимальная сумма должны быть нормализованы до
  integer units и сохраняться неизменяемо после создания.
- Проверяй contract/token, сеть, receiving address, sender/tx identity,
  exact amount, block timestamp и требуемые confirmations/finality. Для TRON
  используй канонический USDT TRC20 contract и подтверждённые TronGrid events;
  для TON — канонический USDT jetton master и trace/finality Toncenter; для
  Solana — канонический USDT mint, legacy SPL token account, owner и
  finalized RPC transaction. Учитывай reorg/late finalization и не считай
  неподтверждённый или просроченный event оплатой.
- Обработчик событий обязан быть идемпотентным по устойчивому blockchain
  идентификатору (tx/log/event), с атомарным settlement и ledger update. Повтор,
  webhook race и параллельные watcher-инстансы не должны удваивать баланс.
- Фиксируй lifecycle (`created`, `seen`, `confirmed`, `settled`, `expired`,
  `failed`) и expiry. Просроченный платёж не переназначай другому пользователю;
  поздние поступления отправляй в безопасный reconciliation/manual review.
- Никогда не делай direct chain invoice успешным вручную, по одному callback или
  админскому флагу (для TRON, TON и Solana одинаково): ручная корректировка
  возможна только как audited reconciliation с проверяемым on-chain proof.
  Exact-amount suffix/identifier после выдачи навсегда занят — не переиспользуй
  его даже после expiry.
- Окно invoice определяется временем блока события (event/block timestamp), а
  не временем обнаружения watcher-ом. Если event подтверждён после expiry,
  сначала докажи, что он был mined до expiry; без проверки invoice не воскрешай.
- Reconciliation продолжай по immutable snapshot (адрес, контракт, окно) для
  pending/expired invoices после отключения метода или ротации адреса; новые
  invoices при этом должны fail closed. Не ограничивай watcher только текущими
  настройками.
- Reconciliation должен находить orphan/mismatch/underpayment/overpayment,
  быть повторяемым и не менять баланс молча. Любая ручная корректировка требует
  audit trail и blockchain proof; admin endpoint не должен позволять прямое
  зачисление без proof.
- API keys и иные секреты держи только на backend/secret storage; не клади их в
  frontend bundle, URL, логи или клиентскую БД. Watcher должен быть read-only.
- Rate limit создания оплат на пользователя настраиваемый; `0` означает
  выключено. Ограничение проверяй атомарно на границе создания, во всех
  инстансах, и не путай с лимитом HTTP/API запросов.

Обязательны тесты на authoritative PayMethods gate, positive/negative settlement,
wrong chain/contract/address, точность integer units, confirmations/time/expiry,
duplicate и concurrent events, late finalization, reconciliation и fail-closed
конфигурацию. Для схемы проверь SQLite, MySQL и PostgreSQL-совместимость и
rollback старого бинарника; для UI — direct-only visibility и i18n всех шести
локалей.
