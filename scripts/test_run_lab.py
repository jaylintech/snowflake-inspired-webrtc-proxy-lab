import argparse
import unittest
from pathlib import Path
from unittest.mock import patch

import run_lab


class TeardownTests(unittest.TestCase):
    @patch.object(Path, "exists", return_value=True)
    @patch.object(run_lab, "command_available", return_value=True)
    @patch.object(run_lab, "role_command", return_value=["labcert", "-clean"])
    @patch.object(run_lab, "run_foreground", side_effect=[0, 0])
    def test_teardown_stops_compose_before_cleaning_certificates(
        self, run_foreground, _role_command, _command_available, _exists
    ):
        result = run_lab.run_teardown(argparse.Namespace(out="private/turn"))
        self.assertEqual(result, 0)
        self.assertEqual(run_foreground.call_count, 2)
        self.assertEqual(run_foreground.call_args_list[0].args[0][0:2], ["docker", "compose"])
        self.assertEqual(run_foreground.call_args_list[1].args[0], ["labcert", "-clean"])

    @patch.object(Path, "exists", return_value=True)
    @patch.object(run_lab, "command_available", return_value=True)
    @patch.object(run_lab, "role_command", return_value=["labcert", "-clean"])
    @patch.object(run_lab, "run_foreground", side_effect=[7, 0])
    def test_teardown_propagates_compose_failure(
        self, _run_foreground, _role_command, _command_available, _exists
    ):
        result = run_lab.run_teardown(argparse.Namespace(out="private/turn"))
        self.assertEqual(result, 7)

    @patch.object(run_lab, "command_available", return_value=False)
    @patch.object(run_lab, "role_command", return_value=["labcert", "-clean"])
    @patch.object(run_lab, "run_foreground", return_value=9)
    def test_teardown_propagates_certificate_cleanup_failure(
        self, _run_foreground, _role_command, _command_available
    ):
        result = run_lab.run_teardown(argparse.Namespace(out="private/turn"))
        self.assertEqual(result, 9)


if __name__ == "__main__":
    unittest.main()
