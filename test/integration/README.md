# Warning

"Integration" tests have a bad history here, because unless you are careful and
clear in your intent, it becomes very difficult to tell what the unit under test
is.

At one point, we had

- some tests compiling their own, separate versions of binaries to test
- some tests relying on ambient PATHs to find binaries to test
- some tests running mock hub/agent servers in-house
- some tests starting up new hub/agent servers out-of-process

and of course, they all did this in the same process space where it was possible
for the separate strategies to overlap and collide. At several points in time,
we have discovered tests that didn't appear to be testing anything but the mock
behavior we had given them.

# Takeaway

Think carefully about where you want to put new tests. Consider the two
extremes:

Correctly-designed unit tests run quickly and give fine control over mock
boundaries. Prefer them for the majority of test coverage, but understand that
they don't provide any confidence that the units actually do anything useful
when put together.

The BATS end-to-end tests are more expensive, but provide true acceptance-level
testing. I.e. they can help answer the question, "Did we put all the units
together correctly?" Many of the old integration tests were much better suited
for the end-to-end style, because they were just calling the binaries anyway.

So that leaves integration in the middle, where

- you're trying to test the combined behavior of multiple units, AND
- end-to-end tests would be too expensive, OR
- end-to-end tests don't give enough control over the test internals, OR
- it's easier to express your test intention in Go than in Bash.

There are probably other reasons to write integration tests. You're smarter than
this README. But be _careful_ when writing them. Make the code under test
explicit, and tightly control the boundaries around that code -- don't allow
tested behavior to escape into the environment. If that's hard to do, consider
pushing the test towards one of the two extremes.

In the end, there will be grey areas. Just do the best you can and continually
improve the tests, and it'll all be good.
