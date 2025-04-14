#!/usr/bin/env python

"""
Simple test script to verify our fix to the ChartSolver 'tuple' has no is_dir attribute issue
"""

import sys
from pathlib import Path

from solver import (
    _process_chart_binary_search_task,
    _process_chart_task,
    get_chart_classification,
)


# Test script to directly call the functions that were causing the tuple error
def test_solver_fix():
    print("Testing ChartSolver fix functions...")

    # Get the path to the test chart
    chart_path = Path("plex-6.4.3.tgz")
    if not chart_path.exists():
        print(
            f"Chart {chart_path} not found. Make sure you're running from the root directory."
        )
        sys.exit(1)

    # Create a tuple as in the original error case
    chart_tuple = ("plex", chart_path)

    # Test the is_dir issue was fixed in various functions

    # 1. Test get_chart_classification
    try:
        print("Testing get_chart_classification...")
        # Try with Path
        classification_path = get_chart_classification(chart_path)
        print(f"  ✓ Path works: {classification_path}")

        # Now try with tuple[1] which was causing the issue
        classification_tuple = get_chart_classification(chart_tuple[1])
        print(f"  ✓ Tuple path works: {classification_tuple}")
    except Exception as e:
        print(f"  ✗ get_chart_classification failed: {e}")
        return False

    # 2. Test _process_chart_task
    try:
        print("Testing _process_chart_task...")
        # Create minimal solver_config
        solver_config = {
            "strategy": "binary",
            "irr_binary": Path("bin/irr"),
            "parameter_matrix": {"kubeVersion": ["1.28.0"]},
            "max_attempts_per_chart": 1,
            "target_registry": "docker.io",
        }

        # Just test the function call without running it fully
        try:
            # Just make sure the function starts without "tuple has no is_dir" error
            _process_chart_task(chart_tuple, solver_config)
            print("  ✓ No immediate is_dir error")
        except Exception as e:
            # It may fail for other reasons like missing binary, but we only care about the is_dir issue
            if "has no attribute 'is_dir'" in str(e):
                print(f"  ✗ _process_chart_task failed with is_dir error: {e}")
                return False
            else:
                print(f"  ✓ Function failed with expected non-is_dir error: {e}")
    except Exception as e:
        print(f"  ✗ _process_chart_task test setup failed: {e}")
        return False

    # 3. Test binary_search_params
    try:
        print("Testing _process_chart_binary_search_task...")
        try:
            # Again, just test that it starts without the is_dir error
            _process_chart_binary_search_task(
                chart_tuple,
                Path("bin/irr"),
                {"kubeVersion": ["1.28.0"]},
                1,
                "docker.io",
            )
            print("  ✓ No immediate is_dir error")
        except Exception as e:
            # It may fail for other reasons
            if "has no attribute 'is_dir'" in str(e):
                print(
                    f"  ✗ _process_chart_binary_search_task failed with is_dir error: {e}"
                )
                return False
            else:
                print(f"  ✓ Function failed with expected non-is_dir error: {e}")
    except Exception as e:
        print(f"  ✗ _process_chart_binary_search_task test setup failed: {e}")
        return False

    # All tests passed!
    return True


if __name__ == "__main__":
    if test_solver_fix():
        print("✅ All tests passed - is_dir attribute error is fixed!")
        sys.exit(0)
    else:
        print("❌ Test failed")
        sys.exit(1)

# Original content below
