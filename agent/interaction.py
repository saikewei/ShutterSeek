"""A small, deterministic contract for generated browser interaction checks."""
import re

ID_SELECTOR = re.compile(r"#[A-Za-z_][A-Za-z0-9_-]{0,298}")


def validate_interaction(item):
    # Stable IDs avoid ambiguous targets and natural-language test expectations.
    if not isinstance(item, dict):
        raise ValueError("interaction must be an object")
    selector = item.get("selector")
    if not isinstance(selector, str) or not ID_SELECTOR.fullmatch(selector):
        raise ValueError("interaction selector must be a stable #id selector")
    expected = item.get("expect_visible")
    if expected is not None and expected != "":
        if not isinstance(expected, str) or not ID_SELECTOR.fullmatch(expected):
            raise ValueError("expect_visible must be a stable #id selector")
    return item
