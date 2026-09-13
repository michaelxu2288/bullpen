# Scenario: Incident Triage

Checkout latency regressed from 320ms to 4s after a deploy. Nobody has written
the postmortem yet; the only record is a Slack thread and the indexed docs.

## 1. Retrieve, before any agent runs

```bash
bullpen context retrieve \
  --query "why did checkout latency regress after the payments deploy" \
  --role reviewer --channels C0INCIDENT,C0BACKEND --top-k 6
```

The graph fans out to both arms in one superstep:

```
embed(hashing:384d)
  -> recall_slack(stub:4)      #incidents thread + the retry reminder
  -> recall_vectors(memory:4)  retry policy, auth rate limits, release notes
  -> fuse(rrf:4v+4s=8)         the doc found by both arms wins
  -> rerank(role:reviewer)     reviewer bias pulls the incident thread up
  -> pack_context(4 chunks)    [S1]..[S4] with citations
```

The reviewer role is what puts the Slack thread above the design doc. Run the
same query with `--role coder` and the indexed retry policy comes first instead.

## 2. Plan against the retrieved context

```bash
bullpen crew --workers 3 --goal "root cause and fix the checkout latency regression"
```

1. Planner decomposes the incident scope, quoting `[S1]` and `[S2]`.
2. Coder drafts the patch and a rollback plan; it calls `pinecone.query` for the
   code-shaped questions, skipping the Slack arm.
3. Reviewer checks blast radius against the policy docs.
4. The HITL gate asks for human approval when confidence is below threshold.

## 3. Write the finding back

```bash
bullpen tools --call index.upsert --params \
  id=incident:2024-08-28,text="429s from the auth service were retried as transient; the payments client now treats them as terminal"
```

The next agent's `recall_vectors` sees it.

## 4. Report back to the channel

```bash
bullpen tools --call slack.mcp --params \
  tool=conversations_add_message,channel=C0INCIDENT,text="rollback verified, fix in review"
```

This goes through the same MCP connection the recall arm used, so it obeys
whatever scopes the Slack MCP server was granted.
