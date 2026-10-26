def native_users_match(users, want):
    """Check current native accounts; a hot-update can leave config.json stale."""
    if sorted(u.get("email", "") for u in users) != sorted(want["emails"]):
        return False
    expected = want.get("vless_users")
    if expected is None:
        return True
    if not isinstance(expected, list) or len(expected) != len(users):
        return False
    def identity(user):
        account = user.get("account")
        if not isinstance(account, dict) or not account.get("id"):
            return None
        return (user.get("email", ""), account["id"], account.get("flow", ""))
    observed = [identity(user) for user in users]
    if any(value is None for value in observed):
        return False
    wanted = [(user["email"], user["id"], user["flow"]) for user in expected]
    return len(set(wanted)) == len(wanted) and sorted(observed) == sorted(wanted)
