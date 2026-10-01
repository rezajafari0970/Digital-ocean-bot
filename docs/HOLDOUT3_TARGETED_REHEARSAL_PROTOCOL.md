# Holdout 3 Targeted Exact-Rehearsal

Holdout 3 is frozen historical evidence. Never alter or rerun it.

Train only exact failures in three sets under `docs/holdout3-targeted-rehearsal/`.

For conceptual cards, memorize the complete structured object: package, previous_symbol, next_symbol, calls, db_reads, db_writes, routes, side_effects. Empty values are meaningful and order of list values is meaningful.

For topology cards, memorize the complete ordered symbols array or exact handler. Empty/partial arrays are failures.

For verbatim cards, exact bytes represented by JSON text matter: tabs/newlines, backticks, quotes, underscores, punctuation, identifiers and line order.

For every card use: READ CORRECTION ONCE → CLOSE → GENERATE EXACTLY → COMPARE ONCE → CLOSE → REGENERATE MISSES ONCE.

After all cards, enter closed-book mode. The next measurement must be a newly generated unseen Holdout 4; never use Holdout 3 as the post-training test.
