### Added
- `desksupervise tick` now reconciles every in-flight dispatch claim's ELIGIBILITY before the
  liveness step: a run whose PR was merged or closed, whose board row flipped off
  `todo`/`in-progress`, or whose claim was released or stolen is STOPPED within one observer
  interval — terminal cases release the claim for re-dispatch, held cases (a
  SUPERSEDED/RESOLVED-ELSEWHERE disposition or a `needs-decision`/`question` label) stop the run
  without releasing it. A reconcile read that could-not-check keeps the run and retries next tick.
  This turns "a merged or closed PR is DONE, stop" from a rule a worker had to remember into a
  mechanical backstop.
