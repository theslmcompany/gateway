import yaml
from transformers import pipeline
import transformers

# Compatibility shim for models whose tokenizer_config.json names the
# transformers v5 class "TokenizersBackend". No-op on transformers >= 5, which
# provides the class natively.
if not hasattr(transformers, "TokenizersBackend"):
    transformers.TokenizersBackend = transformers.PreTrainedTokenizerFast


def _build_pipeline(spec: dict):
    model = spec["model"]
    task = spec["task"]
    device = spec.get("device", -1)
    use_fast = spec.get("use_fast", True)

    # trust_remote_code executes arbitrary Python shipped in the model repo,
    # inside this sidecar, at load time. It defaults OFF: a model that needs it
    # must say so explicitly, and only after its custom code has been reviewed.
    trust = spec.get("trust_remote_code", False)

    # revision pins the model to an immutable commit SHA. Without it, `main` is
    # a moving target — whoever controls the repo (or anyone who compromises
    # that account) can swap the artefact we load on the next image build.
    revision = spec.get("revision")

    if trust:
        print(
            f"[mlrunner] WARNING: trust_remote_code=true for {model} — "
            "arbitrary code from the model repo will execute in this process"
        )
    if revision is None:
        print(
            f"[mlrunner] WARNING: no revision pinned for {model} — "
            "loading a mutable ref, artefact may change between builds"
        )

    common = {"trust_remote_code": trust}
    if revision:
        common["revision"] = revision

    try:
        from transformers import AutoTokenizer
        tokenizer = AutoTokenizer.from_pretrained(
            model,
            use_fast=use_fast,
            extra_special_tokens={},  # Workaround for 'list' object has no attribute 'keys'
            **common,
        )
    except Exception as e:
        print(f"[mlrunner] Failed to initialize tokenizer manually for {model}: {e}")
        tokenizer = None

    if tokenizer is not None:
        return pipeline(
            task,
            model=model,
            tokenizer=tokenizer,
            device=device,
            **common,
        )
    else:
        return pipeline(
            task,
            model=model,
            device=device,
            use_fast=use_fast,
            **common,
        )


def load_registry(config_path: str = "analyzers.yml") -> dict:
    """Load analyzer pipelines from YAML config. Called once at startup."""
    with open(config_path) as f:
        cfg = yaml.safe_load(f)

    registry = {}
    for name, spec in cfg.get("analyzers", {}).items():
        pipe = _build_pipeline(spec)
        registry[name] = {
            "pipeline": pipe,
            "max_length": spec.get("max_length", 512),
            "task": spec["task"],
        }
        print(f"[mlrunner] loaded analyzer: {name} ({spec['model']})")

    return registry
