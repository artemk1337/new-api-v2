package setting

var NOWPaymentsEnabled = false
var NOWPaymentsAPIKey = ""
var NOWPaymentsIPNSecret = ""

// Kept as legacy option keys, but NOWPayments top-ups always settle in USDT.
var NOWPaymentsPriceCurrency = "usdt"
var NOWPaymentsPayCurrency = "usdt"
var NOWPaymentsIPNCallbackURL = ""
