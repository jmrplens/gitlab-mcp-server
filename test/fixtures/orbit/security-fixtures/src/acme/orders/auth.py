"""Auth helpers — every function in this module has a security weakness
on purpose so the SAST analyzer emits findings."""

import hashlib
from typing import Optional


def verify_password_weak(stored_hash: str, candidate: str) -> bool:
    """Compare a stored password hash to a candidate using the broken
    `==` pattern on a `md5(...).hexdigest()` value. Real code must use
    a password hasher like bcrypt or argon2."""
    # INTENTIONAL WEAK HASH for fixture SAST detections.
    candidate_hash = hashlib.md5(candidate.encode("utf-8")).hexdigest()  # nosec
    return stored_hash == candidate_hash


def build_query_naive(user_input: str) -> str:
    """Return a SQL fragment built by string concatenation — DO NOT
    copy this pattern into real code."""
    # INTENTIONAL SQL INJECTION VECTOR for fixture SAST detections.
    return f"SELECT * FROM users WHERE name = '{user_input}'"


def eval_unsafe(expression: str) -> Optional[float]:
    """Evaluate a mathematical expression passed in as a string. This
    looks like a calculator helper but the use of `eval` makes it a
    remote code execution sink."""
    # INTENTIONAL eval() USE for fixture SAST detections.
    return eval(expression, {"__builtins__": {}}, {})  # nosec
