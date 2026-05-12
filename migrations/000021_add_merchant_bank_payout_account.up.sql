INSERT INTO paygate_ledger.ledger_accounts (code, name, type, description)
VALUES (
    'MERCHANT_BANK_PAYOUT',
    'Merchant bank payout',
    'asset',
    'Cash outflow to merchant bank accounts during payout completion'
)
ON CONFLICT (code) DO NOTHING;
