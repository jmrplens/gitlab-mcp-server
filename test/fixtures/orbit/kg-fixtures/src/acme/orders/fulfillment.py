"""Fulfillment helpers — touch the `Order` type from models so the indexer
records cross-module references via `ImportedSymbol`."""

from typing import Dict

from acme.orders.models import Order, OrderStatus


# In-memory stock register. Realistic fixtures would persist this in a DB.
_STOCK: Dict[str, int] = {}


def seed_stock(sku: str, quantity: int) -> None:
    """Add `quantity` units of `sku` to the in-memory stock register."""
    _STOCK[sku] = _STOCK.get(sku, 0) + quantity


def reserve_stock(order: Order) -> None:
    """Decrement the in-memory stock count for every item in the order.

    Raises `ValueError` if any SKU is out of stock or unknown.
    """
    for item in order.items:
        available = _STOCK.get(item.sku, 0)
        if available < item.quantity:
            raise ValueError(f"insufficient stock for {item.sku} (requested {item.quantity}, have {available})")
        _STOCK[item.sku] = available - item.quantity


def schedule_pickup(order: Order, pickup_window: str) -> str:
    """Mark the order as CONFIRMED and return a synthetic pickup reference."""
    order.status = OrderStatus.CONFIRMED
    return f"PU-{order.order_id}-{pickup_window}"


def mark_shipped(order: Order, tracking_number: str) -> None:
    """Move the order to SHIPPED and store the tracking number in notes."""
    order.status = OrderStatus.SHIPPED
    order.notes = f"tracking={tracking_number}"
