"""Domain models for the orders package.

The classes here are pure data containers. The Knowledge Graph indexes
each class and its dunder methods as separate `Definition` entities.
"""

from dataclasses import dataclass, field
from enum import Enum


class OrderStatus(str, Enum):
    """Lifecycle states for an order."""

    PENDING = "pending"
    CONFIRMED = "confirmed"
    SHIPPED = "shipped"
    DELIVERED = "delivered"
    CANCELLED = "cancelled"


@dataclass
class OrderItem:
    """A single line item inside an order."""

    sku: str
    quantity: int
    unit_price_cents: int

    def line_total_cents(self) -> int:
        """Return the line total in cents (no tax, no discount)."""
        return self.quantity * self.unit_price_cents


@dataclass
class Order:
    """An order placed by a customer."""

    order_id: str
    customer_id: str
    items: list[OrderItem] = field(default_factory=list)
    status: OrderStatus = OrderStatus.PENDING
    notes: str | None = None

    def add_item(self, item: OrderItem) -> None:
        """Append a line item to this order."""
        self.items.append(item)

    def cancel(self, reason: str) -> None:
        """Cancel the order and record the reason in notes."""
        self.status = OrderStatus.CANCELLED
        self.notes = reason
