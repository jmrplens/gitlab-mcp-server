"""Minimal pytest coverage for the pricing helpers."""

import pytest

from acme.orders.models import OrderItem
from acme.orders.pricing import compute_subtotal, apply_discount, compute_total


def _item(qty: int, price_cents: int) -> OrderItem:
    return OrderItem(sku="SKU-1", quantity=qty, unit_price_cents=price_cents)


def test_compute_subtotal_sums_line_totals():
    items = [_item(2, 500), _item(1, 1000)]
    assert compute_subtotal(items) == 2000


def test_apply_discount_clamps_to_zero_and_hundred():
    assert apply_discount(1000, -10) == 1000
    assert apply_discount(1000, 150) == 0
    assert apply_discount(1000, 25) == 750


def test_compute_total_with_discount_and_tax():
    items = [_item(3, 200)]
    # subtotal=600, discount 10% -> 540, tax 50 -> 590
    assert compute_total(items, discount_percent=10.0, tax_cents=50) == 590


@pytest.mark.parametrize("discount,expected", [(0, 1000), (10, 900), (100, 0)])
def test_compute_total_discount_parametrized(discount, expected):
    items = [_item(2, 500)]
    assert compute_total(items, discount_percent=discount) == expected
