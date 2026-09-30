Feature: The stack survives memory pressure

  On 2026-09-29 the Docker machine ran out of memory. The test runners in two sessions held 15.4
  gibibytes between them. The kernel picks what to kill by size and by each process's kill priority,
  every container in the stack carried the same priority of zero, and the thing it killed was the
  control plane, at about 110 mebibytes. Every exec that was running died with "the system restarted
  while this exec was running". The work in every session went with it.

  Later the same day the Docker machine stopped and started again. No service in the compose file
  asked to be started again, so the whole stack stayed down until somebody started each container by
  hand.

  So the control plane and the database are the last two things the kernel takes, and every service
  comes back on its own. A session may still be killed, and that is the point: one session loses its
  exec, the system stays up, and the operator can see what happened.

  This scenario reads the compose file the operator's stack starts from. It cannot show the kernel
  applying the priority, because that needs a running daemon. The containers job in continuous
  integration boots the stack for real.

  Scenario: The compose file gives the control plane and the database a kill priority of -900, and gives every service the restart policy unless-stopped.
    When the operator reads the compose file the stack starts from
    Then the "controlplane" service carries a kill priority of -900, so the kernel takes a session first
    And the "postgres" service carries a kill priority of -900, so the kernel takes a session first
    And every service starts again by itself after the machine restarts
