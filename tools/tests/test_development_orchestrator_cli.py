import contextlib
import importlib.util
import io
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


class OrchestratorCLI(unittest.TestCase):
    def setUp(self):
        path = Path(__file__).resolve().parents[1] / 'development-orchestrator.py'
        spec = importlib.util.spec_from_file_location('orchestrator_cli', path)
        self.module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.module)

    def test_diagnostics_and_invalid_inputs_never_enter_job(self):
        for args, env, code in [(['--help'], {}, 0), (['--invalid'], {}, 2),
                                 ([], {}, 2), (['--job', '../escape'], {}, 2),
                                 (['--job', 'missing'], {}, 2),
                                 (['--help'], {'DEV_JOB': 'old-job'}, 0)]:
            with self.subTest(args=args, env=env), tempfile.TemporaryDirectory() as tmp:
                with patch.dict(os.environ, env, clear=True), patch.object(self.module, 'JOBS', Path(tmp)), \
                     patch.object(self.module, 'one') as run, patch.object(self.module, 'state') as state, \
                     contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                    with self.assertRaises(SystemExit) as exit:
                        self.module.main(args)
                    self.assertEqual(exit.exception.code, code)
                    run.assert_not_called()
                    state.assert_not_called()
                    self.assertEqual(list(Path(tmp).iterdir()), [])

    def test_explicit_job_and_scheduled_environment_remain_supported(self):
        for args, env in [(['--job', 'chosen-job'], {}), ([], {'DEV_JOB': 'chosen-job'})]:
            with tempfile.TemporaryDirectory() as tmp:
                (Path(tmp) / 'chosen-job.json').write_text('{}')
                with patch.dict(os.environ, env, clear=True), patch.object(self.module, 'JOBS', Path(tmp)), \
                     patch.object(self.module, 'one', return_value=0) as run, \
                     patch.object(self.module, 'state', return_value={'jobs': {'chosen-job': {'phase': 'COMPLETE'}}}):
                    self.module.main(args)
                    run.assert_called_once_with('chosen-job')


if __name__ == '__main__':
    unittest.main()
