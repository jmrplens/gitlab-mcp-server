"""Pricing helpers — small enough to read in one pass but rich enough for
the indexer to populate several `Definition` entities and a few
`ImportedSymbol` cross-references (OrderItem is imported from models)."""

from typing import Iterable, Optional

from acme.orders.models import OrderItem


def compute_subtotal(items: Iterable[OrderItem]) -> int:
    """Return the sum of line totals in cents."""
    return sum(item.line_total_cents() for item in items)


def apply_discount(subtotal_cents: int, percent: float) -> int:
    """Apply a percentage discount and return the discounted subtotal in cents.

    Negative `percent` values are treated as zero (no negative discounts).
    """
    if percent < 0:
        percent = 0.0
    if percent > 100:
        percent = 100.0
    return int(round(subtotal_cents * (1 - percent / 100)))


def compute_total(items: Iterable[OrderItem], discount_percent: float = 0.0, tax_cents: Optional[int] = None) -> int:
    """Compute the final order total in cents: subtotal - discount + tax.

    When `tax_cents` is None, tax defaults to zero.
    """
    subtotal = compute_subtotal(items)
    discounted = apply_discount(subtotal, discount_percent)
    tax = tax_cents if tax_cents is not None else 0
    return discounted + tax
