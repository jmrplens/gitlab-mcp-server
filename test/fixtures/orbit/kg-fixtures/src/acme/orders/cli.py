"""Tiny CLI entry point — exercises every other module end-to-end so the
`Definition` count and the `ImportedSymbol` edges show up clearly in
the Knowledge Graph."""

from __future__ import annotations

import argparse
import sys

from acme.orders import __version__
from acme.orders.models import Order, OrderItem, OrderStatus
from acme.orders.pricing import compute_total
from acme.orders.fulfillment import reserve_stock, schedule_pickup, mark_shipped


def parse_args(argv: list[str]) -> argparse.Namespace:
    """Parse the minimal CLI surface."""
    parser = argparse.ArgumentParser(prog="acme-orders", description="Acme order-processing CLI")
    parser.add_argument("--order-id", required=True)
    parser.add_argument("--customer-id", required=True)
    parser.add_argument("--sku", action="append", required=True, help="Repeatable SKU flag")
    parser.add_argument("--quantity", action="append", type=int, required=True, help="Repeatable quantity flag")
    parser.add_argument("--unit-price-cents", action="append", type=int, required=True, help="Repeatable unit price")
    parser.add_argument("--discount-percent", type=float, default=0.0)
    parser.add_argument("--tax-cents", type=int, default=0)
    parser.add_argument("--version", action="version", version=f"acme-orders {__version__}")
    return parser.parse_args(argv)


def build_order(args: argparse.Namespace) -> Order:
    """Translate parsed CLI args into a fully populated `Order`.

    Rejects mismatched repeated flags (e.g. two SKUs but only one
    quantity) with a clear error rather than silently dropping the
    extra SKU via `zip`.
    """
    sku = list(args.sku)
    qty = list(args.quantity)
    price = list(args.unit_price_cents)
    if not (len(sku) == len(qty) == len(price)):
        raise SystemExit(
            f"--sku, --quantity, and --unit-price-cents must be repeated the "
            f"same number of times (got {len(sku)} SKU(s), {len(qty)} quantity, "
            f"{len(price)} unit-price-cents)"
        )
    order = Order(order_id=args.order_id, customer_id=args.customer_id)
    for s, q, p in zip(sku, qty, price):
        order.add_item(OrderItem(sku=s, quantity=q, unit_price_cents=p))
    return order


def main(argv: list[str] | None = None) -> int:
    """Run the full fulfillment pipeline and print a one-line summary."""
    args = parse_args(sys.argv[1:] if argv is None else argv)
    order = build_order(args)
    total = compute_total(order.items, args.discount_percent, args.tax_cents)
    reserve_stock(order)
    schedule_pickup(order, pickup_window="today")
    mark_shipped(order, tracking_number="TRK-DEMO-001")
    print(f"order={order.order_id} status={order.status.value} total_cents={total}")
    return 0 if order.status is OrderStatus.SHIPPED else 1


if __name__ == "__main__":
    raise SystemExit(main())
