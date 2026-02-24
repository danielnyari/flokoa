import re


def _to_snake_case(text: str) -> str:
    """Converts a string into snake_case.

    Handles lowerCamelCase, UpperCamelCase, or space-separated case, acronyms
    (e.g., "REST API") and consecutive uppercase letters correctly.  Also handles
    mixed cases with and without spaces.

    Examples:
    ```
    to_snake_case('camelCase') -> 'camel_case'
    to_snake_case('UpperCamelCase') -> 'upper_camel_case'
    to_snake_case('space separated') -> 'space_separated'
    ```

    Args:
        text: The input string.

    Returns:
        The snake_case version of the string.
    """

    # Handle spaces and non-alphanumeric characters (replace with underscores)
    text = re.sub(r"[^a-zA-Z0-9]+", "_", text)

    # Insert underscores before uppercase letters (handling both CamelCases)
    text = re.sub(r"([a-z0-9])([A-Z])", r"\1_\2", text)  # lowerCamelCase
    text = re.sub(r"([A-Z]+)([A-Z][a-z])", r"\1_\2", text)  # UpperCamelCase and acronyms

    # Convert to lowercase
    text = text.lower()

    # Remove consecutive underscores (clean up extra underscores)
    text = re.sub(r"_+", "_", text)

    # Remove leading and trailing underscores
    text = text.strip("_")

    return text
