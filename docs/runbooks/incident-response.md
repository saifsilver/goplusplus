# Incident response runbook

1. Assign an incident lead and record start time, affected versions, and impact.
2. Protect users first: disable the failing capability, reduce traffic, or roll
   back to the preceding verified release.
3. Preserve sanitized logs, traces, metrics, deployment identifiers, and
   database/broker state. Never copy credentials into the incident record.
4. Determine whether integrity, confidentiality, availability, or tenant
   isolation is affected. Follow `SECURITY.md` for suspected vulnerabilities.
5. Communicate verified facts, mitigation status, and the next update time.
6. Validate recovery through user-visible checks and leading indicators.
7. Produce a blameless review with root cause, contributing conditions,
   detection gaps, corrective owners, and deadlines.
