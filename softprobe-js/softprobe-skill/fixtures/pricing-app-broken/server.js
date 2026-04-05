'use strict';
const express = require('express');

const PORT = parseInt(process.env.PORT ?? '3020', 10);
const TAX_RATE = 0.08;

function computePriceCents(base) {
  return base + Math.round(base * TAX_RATE);
}

function basePriceForSku(sku) {
  return sku === 'coffee-beans' ? 1000 : 500;
}

const app = express();

app.get('/ping', (_req, res) => res.send('ok'));

app.get('/price', (req, res) => {
  const sku = String(req.query.sku ?? 'coffee-beans');
  const base = basePriceForSku(sku);
  const priceCents = computePriceCents(base);
  res.json({
    sku,
    basePriceCents: base,
    priceCents,
    price: `$${(priceCents / 100).toFixed(2)}`,
  });
});

app.listen(PORT, () =>
  console.log(`pricing-app listening on http://127.0.0.1:${PORT}`)
);
