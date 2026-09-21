# TestHive — state and reset

TestHive keeps its login and cart **in memory**. A plain relaunch already
returns it to the login screen with an empty cart, so `launch_app` (fresh by
default) is a real clean slate — there is nothing on disk to wipe. Do not
assert a reset by checking the login screen alone (a relaunch shows it either
way); assert it by a container the previous session filled being empty.
