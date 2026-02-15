import json
import os

from flokoa.types import TemplateConfig

TEMPLATE_CONFIG_PATH = "/etc/flokoa/template-config.json"


def load_templated_config() -> TemplateConfig:
    """Load templated agent configuration from /etc/flokoa/template-config.json.

    Returns:
        TemplateConfig parsed from the config file.

    Raises:
        FileNotFoundError: If the config file does not exist.
    """
    path = os.environ.get("FLOKOA_TEMPLATE_CONFIG_PATH", TEMPLATE_CONFIG_PATH)
    if not os.path.exists(path):
        raise FileNotFoundError(f"Templated config file not found at {path}")

    with open(path) as f:
        config_data = json.load(f)

    return TemplateConfig.model_validate(config_data)
