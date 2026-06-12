"""Smoke tests for the fulfillment helpers."""

import pytest

from acme.orders.models import Order, OrderItem, OrderStatus
from acme.orders import fulfillment


@pytest.fixture
def empty_order() -> Order:
    return Order(order_id="O-1", customer_id="C-1")


def test_reserve_stock_rejects_unknown_sku(empty_order):
    empty_order.add_item(OrderItem(sku="NOPE", quantity=1, unit_price_cents=100))
    with pytest.raises(ValueError, match="insufficient stock"):
        fulfillment.reserve_stock(empty_order)


def test_full_pipeline_transitions_status(empty_order):
    empty_order.add_item(OrderItem(sku="OK", quantity=1, unit_price_cents=100))
    fulfillment._STOCK["OK"] = 5  # seed the in-memory stock
    fulfillment.reserve_stock(empty_order)
    fulfillment.schedule_pickup(empty_order, pickup_window="today")
    fulfillment.mark_shipped(empty_order, tracking_number="T-1")
    assert empty_order.status is OrderStatus.SHIPPED
    assert empty_order.notes == "tracking=T-1"
