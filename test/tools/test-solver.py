#!/usr/bin/env python3

import argparse
import json
import random
import sys
from pathlib import Path

from solver import ChartSolver


# Define simulation wrapper outside main function to make it picklable
def simulate_errors_wrapper(chart_path, simulation_data):
    """Wrapper function to simulate errors based on the simulation data."""
    if str(chart_path) in simulation_data:
        error_data = simulation_data[str(chart_path)]
        return {
            "chart_name": Path(chart_path).name,
            "classification": "STANDARD",
            "path": str(chart_path),
            "tested_params": [
                {
                    "params": {},
                    "status": error_data["status"],
                    "category": error_data["category"],
                    "details": error_data["details"],
                }
            ],
            "successful_params": [],
            "minimal_params": None,
            "error_categories": {error_data["category"]: 1},
        }
    else:
        # Process normally (this will be handled by the solver)
        return None


def main():
    parser = argparse.ArgumentParser(
        description="Test the ChartSolver with a small set of charts"
    )
    parser.add_argument(
        "--charts-dir",
        type=str,
        required=True,
        help="Directory containing charts to test",
    )
    parser.add_argument(
        "--output-dir",
        type=str,
        default="./solver_output",
        help="Output directory for results",
    )
    parser.add_argument(
        "--workers", type=int, default=4, help="Number of parallel workers"
    )
    parser.add_argument("--debug", action="store_true", help="Enable debug output")
    parser.add_argument("--limit", type=int, help="Limit number of charts to process")
    parser.add_argument(
        "--resume", action="store_true", help="Resume from previous checkpoint"
    )
    parser.add_argument(
        "--simulate-errors",
        action="store_true",
        help="Simulate random errors for testing",
    )
    parser.add_argument(
        "--error-rate",
        type=float,
        default=0.3,
        help="Percentage of charts that will simulate errors (0.0-1.0)",
    )
    args = parser.parse_args()

    # Setup paths
    charts_dir = Path(args.charts_dir)
    output_dir = Path(args.output_dir)
    output_dir.mkdir(parents=True, exist_ok=True)

    if not charts_dir.exists():
        print(f"Error: Charts directory '{charts_dir}' does not exist.")
        return 1

    # Discover charts
    chart_paths = []
    for chart_path in charts_dir.glob("**/*.tgz"):
        chart_paths.append(chart_path)

    if args.limit and args.limit > 0:
        chart_paths = chart_paths[: args.limit]

    print(f"Found {len(chart_paths)} charts to process")

    # Initialize solver
    solver = ChartSolver(
        max_workers=args.workers,
        output_dir=output_dir,
        debug=args.debug,
        checkpoint_interval=10,  # Save checkpoint every 10 charts
    )

    # Setup error simulation if requested
    if args.simulate_errors:
        # Create error simulation file
        error_types = [
            "KUBE_VERSION_ERROR",
            "REQUIRED_VALUE_ERROR",
            "REGISTRY_ERROR",
            "STORAGE_ERROR",
            "AUTH_ERROR",
        ]

        num_errors = int(len(chart_paths) * args.error_rate)
        print(
            f"Simulating errors for {num_errors} charts ({args.error_rate * 100:.1f}% error rate)"
        )

        # Randomly select charts to simulate errors for
        error_indices = random.sample(range(len(chart_paths)), num_errors)

        # Create simulation data
        simulation_data = {}
        for i in error_indices:
            chart_path = str(chart_paths[i])
            error_type = random.choice(error_types)
            error_msg = f"Simulated {error_type} for testing"

            if error_type == "KUBE_VERSION_ERROR":
                error_msg = "Chart requires Kubernetes version >= 1.21.0"
            elif error_type == "REQUIRED_VALUE_ERROR":
                error_msg = "Required value 'global.registry' is missing"
            elif error_type == "REGISTRY_ERROR":
                error_msg = "Cannot pull image from registry: access denied"
            elif error_type == "STORAGE_ERROR":
                error_msg = "StorageClass 'default' not found"
            elif error_type == "AUTH_ERROR":
                error_msg = "Authentication required to access the registry"

            simulation_data[chart_path] = {
                "status": "ERROR",
                "category": error_type,
                "details": error_msg,
            }

        # Write simulation data to file
        simulation_file = output_dir / "error_simulation.json"
        with open(simulation_file, "w") as f:
            json.dump(simulation_data, f, indent=2)

        print(f"Error simulation data saved to: {simulation_file}")

        # Create a real test run with a random sample of charts with no errors simulated
        # This will give us a mix of successful and error results
        selected_charts = []
        for i, chart_path in enumerate(chart_paths):
            if i not in error_indices:
                selected_charts.append(chart_path)

                # Only select a small subset of charts to run real tests
                if len(selected_charts) >= 5:
                    break

        # Run solver only on selected charts
        if selected_charts:
            print(f"Running actual tests on {len(selected_charts)} charts")
            output_file = output_dir / "solver_real_results.json"
            solver.solve(chart_paths=selected_charts, output_file=output_file)

            # Load real results
            with open(output_file, "r") as f:
                real_results = json.load(f)
        else:
            real_results = {"charts": {}}

        # Create combined results with real and simulated data
        combined_results = (
            real_results.copy() if "charts" in real_results else {"charts": {}}
        )

        # Add simulated error results
        for chart_path, error_data in simulation_data.items():
            combined_results["charts"][chart_path] = {
                "chart_name": Path(chart_path).name,
                "classification": "STANDARD",
                "path": chart_path,
                "tested_params": [
                    {
                        "params": {},
                        "status": error_data["status"],
                        "category": error_data["category"],
                        "details": error_data["details"],
                    }
                ],
                "successful_params": [],
                "minimal_params": None,
                "error_categories": {error_data["category"]: 1},
            }

        # Update summary statistics
        total_charts = len(combined_results["charts"])
        successful_charts = sum(
            1
            for r in combined_results["charts"].values()
            if "minimal_params" in r and r["minimal_params"] is not None
        )

        error_categories = {}
        for chart_data in combined_results["charts"].values():
            if "error_categories" in chart_data:
                for category, count in chart_data["error_categories"].items():
                    error_categories[category] = (
                        error_categories.get(category, 0) + count
                    )

        combined_results["summary"] = {
            "total_charts": total_charts,
            "successful_charts": successful_charts,
            "failed_charts": total_charts - successful_charts,
            "error_categories": error_categories,
        }

        # Save combined results
        combined_output_file = output_dir / "solver_results.json"
        with open(combined_output_file, "w") as f:
            json.dump(combined_results, f, indent=2)

        solver_results = combined_results["charts"]
        print(f"Combined results saved to: {combined_output_file}")
    else:
        # Run solver normally
        output_file = output_dir / "solver_results.json"
        print(f"Starting solver with {args.workers} workers...")

        # Make the simulation_data available to the solver for actual run (empty in this case)
        simulation_data = {}

        solver_results = solver.solve(chart_paths=chart_paths, output_file=output_file)

    # Print summary
    total_charts = len(solver_results)
    successful_charts = sum(
        1
        for r in solver_results.values()
        if "minimal_params" in r and r["minimal_params"] is not None
    )
    success_rate = (successful_charts / total_charts) * 100 if total_charts > 0 else 0

    print("\nSolver Results:")
    print(f"Total charts processed: {total_charts}")
    print(f"Charts successfully solved: {successful_charts} ({success_rate:.2f}%)")

    # Count error categories
    error_categories = {}
    for chart_data in solver_results.values():
        if "minimal_params" not in chart_data or chart_data["minimal_params"] is None:
            if "tested_params" in chart_data and chart_data["tested_params"]:
                category = chart_data["tested_params"][0].get(
                    "category", "UNKNOWN_ERROR"
                )
                error_categories[category] = error_categories.get(category, 0) + 1

    if error_categories:
        print("\nError categories:")
        for category, count in sorted(
            error_categories.items(), key=lambda x: x[1], reverse=True
        ):
            print(f"  {category}: {count} charts ({count / total_charts * 100:.2f}%)")

    # Print example minimal parameter sets
    if successful_charts > 0:
        print("\nExample minimal parameter sets:")
        count = 0
        for chart_path, result in solver_results.items():
            if "minimal_params" in result and result["minimal_params"] is not None:
                print(f"\n{chart_path}:")
                for param_name, param_value in result["minimal_params"].items():
                    print(f"  {param_name}: {param_value}")
                count += 1
                if count >= 3:  # Show at most 3 examples
                    break

    output_file = output_dir / "solver_results.json"
    print(f"Detailed results saved to: {output_file}")

    # Return success
    return 0


if __name__ == "__main__":
    sys.exit(main())
