"""Smoke tests for the security fixture.

The point of this module is to make the CI pipeline include the `test`
stage succeed, so the security templates' `analyzer` jobs run and
populate the security domain of the Orbit knowledge graph.
"""

from acme.orders import fetch_order_unsafe, verify_password_weak, build_query_naive


def test_verify_password_rejects_wrong_candidate():
    assert not verify_password_weak("0" * 32, "hunter2")


def test_build_query_naive_inserts_user_input():
    # Confirms the naive query helper actually embeds the user input —
    # the security scanner looks for this pattern.
    sql = build_query_naive("alice")
    assert "alice" in sql
