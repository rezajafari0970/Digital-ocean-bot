#!/usr/bin/env python3
"""Called under the exclusive deploy lock, after every runtime reader stops."""
import json, pathlib, runpy, subprocess, sys

def check(query, supported):
    if supported:
        return
    if query("SELECT to_regclass('residential_performance_panels') IS NULL") != 't':
        if query("SELECT count(*) FROM residential_performance_panels WHERE COALESCE(jsonb_array_length(config->'excluded_proxy_ids'),0)>0") != '0':
            raise RuntimeError('Unrestored admission assignment: incompatible release denied')
    if query("SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='residential_performance_experiments' AND column_name='tuning')") == 't':
        if query("SELECT count(*) FROM residential_performance_experiments WHERE tuning->>'phase' IN('TESTING','PUBLISHING','RESTORING')") != '0':
            raise RuntimeError('Unresolved tuning: incompatible release denied')

if __name__ == '__main__':
    try:
        supported = json.loads(pathlib.Path(sys.argv[3]).read_text()).get('residential_admission_schema') == 1
        if not supported:
            env = runpy.run_path(sys.argv[1])['read_environment'](pathlib.Path(sys.argv[2]))
            def query(sql):
                p = subprocess.run(['psql',env['DATABASE_URL'],'-XAt','-v','ON_ERROR_STOP=1','-c',sql],capture_output=True,text=True,timeout=10)
                if p.returncode:
                    raise RuntimeError('Compatibility state unavailable')
                return p.stdout.strip()
            check(query, supported)
    except Exception:
        raise SystemExit('Release denied: admission/tuning compatibility is unproven; retain current artifacts')
