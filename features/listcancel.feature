Feature: The control plane never jams on a session list

  A console asks the system for its session list every few seconds. Nobody waits for a list that is
  already late, so the client gives up and asks again. The call it gave up on keeps working.

  On 2026-09-29 this system held 97 session lists and 80 usage calls at the same moment. One list
  took 21 minutes. Nothing could attach and nothing could list while they ran, and almost none of
  them had a caller left to answer.

  Work that nobody will read is work taken from the work somebody waits for. So a call stops where
  its caller stopped.

  Background:
    Given a running control plane
    And a workspace named "acme"
    And a project named "house-bills"

  Scenario: A session list stops when its caller has gone
    Given 40 sessions in the system
    And a caller that gives up part of the way through the list
    When the operator asks for the session list
    Then the list comes back saying its caller went
    And the system read no more sessions after the caller went

  Scenario: A session list answers in full for a caller that waits
    Given 40 sessions in the system
    When the operator asks for the session list
    Then the list holds every one of the 40 sessions
