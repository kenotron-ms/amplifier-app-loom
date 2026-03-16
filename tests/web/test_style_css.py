"""
Tests for web/style.css — validates that the log panel, live badge,
and run card v2 CSS rules are present and correctly structured.
"""
import re
import pytest
from pathlib import Path

CSS_PATH = Path(__file__).parent.parent.parent / "web" / "style.css"


@pytest.fixture(scope="module")
def css_text():
    return CSS_PATH.read_text()


# ---------------------------------------------------------------------------
# .run-card — updated rule
# ---------------------------------------------------------------------------

class TestRunCard:
    def test_display_block(self, css_text):
        """run-card must use display: block (not flex)."""
        block = re.search(
            r'\.run-card\s*\{[^}]*display\s*:\s*block', css_text
        )
        assert block, ".run-card must have 'display: block'"

    def test_no_display_flex(self, css_text):
        """run-card must NOT have display: flex at the top-level rule."""
        # Extract the first .run-card block (not .run-card.live etc.)
        m = re.search(r'(?<![.\w])\.run-card\s*\{([^}]*)\}', css_text)
        assert m, ".run-card rule not found"
        block_content = m.group(1)
        assert 'display: flex' not in block_content, \
            ".run-card must not use 'display: flex'"

    def test_padding_zero(self, css_text):
        """run-card padding must be 0."""
        m = re.search(r'(?<![.\w])\.run-card\s*\{([^}]*)\}', css_text)
        assert m, ".run-card rule not found"
        block_content = m.group(1)
        assert re.search(r'padding\s*:\s*0\b', block_content), \
            ".run-card must have 'padding: 0'"

    def test_overflow_hidden(self, css_text):
        """run-card must have overflow: hidden."""
        m = re.search(r'(?<![.\w])\.run-card\s*\{([^}]*)\}', css_text)
        assert m, ".run-card rule not found"
        block_content = m.group(1)
        assert re.search(r'overflow\s*:\s*hidden', block_content), \
            ".run-card must have 'overflow: hidden'"


# ---------------------------------------------------------------------------
# New rules appended after .onboarding-notice a
# ---------------------------------------------------------------------------

class TestNewRulesOrder:
    def test_new_rules_after_onboarding_notice_a(self, css_text):
        """All new rules must appear after .onboarding-notice a."""
        onboarding_pos = css_text.find('.onboarding-notice a')
        assert onboarding_pos != -1, ".onboarding-notice a rule not found"
        run_header_pos = css_text.find('.run-header', onboarding_pos)
        assert run_header_pos != -1, \
            ".run-header must appear after .onboarding-notice a"


class TestRunHeader:
    def test_run_header_exists(self, css_text):
        assert '.run-header' in css_text

    def test_run_header_flex(self, css_text):
        m = re.search(r'\.run-header\s*\{([^}]*)\}', css_text)
        assert m, ".run-header rule not found"
        content = m.group(1)
        assert re.search(r'display\s*:\s*flex', content)

    def test_run_header_padding(self, css_text):
        m = re.search(r'\.run-header\s*\{([^}]*)\}', css_text)
        assert m
        content = m.group(1)
        assert re.search(r'padding\s*:\s*10px\s+12px', content)

    def test_run_header_gap(self, css_text):
        m = re.search(r'\.run-header\s*\{([^}]*)\}', css_text)
        assert m
        content = m.group(1)
        assert re.search(r'gap\s*:\s*10px', content)


class TestRunName:
    def test_run_name_exists(self, css_text):
        assert '.run-name' in css_text

    def test_run_name_font_weight(self, css_text):
        m = re.search(r'\.run-name\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'font-weight\s*:\s*600', m.group(1))

    def test_run_name_ellipsis(self, css_text):
        m = re.search(r'\.run-name\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'text-overflow\s*:\s*ellipsis', m.group(1))

    def test_run_name_nowrap(self, css_text):
        m = re.search(r'\.run-name\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'white-space\s*:\s*nowrap', m.group(1))


class TestRunTime:
    def test_run_time_exists(self, css_text):
        assert '.run-time' in css_text

    def test_run_time_nowrap(self, css_text):
        m = re.search(r'\.run-time\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'white-space\s*:\s*nowrap', m.group(1))

    def test_run_time_font_size(self, css_text):
        m = re.search(r'\.run-time\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'font-size\s*:\s*11px', m.group(1))


class TestLogToggle:
    def test_log_toggle_exists(self, css_text):
        assert '.log-toggle' in css_text

    def test_log_toggle_no_decoration(self, css_text):
        m = re.search(r'\.log-toggle\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'text-decoration\s*:\s*none', m.group(1))


class TestRunCardLive:
    def test_run_card_live_exists(self, css_text):
        assert '.run-card.live' in css_text

    def test_run_card_live_border_color(self, css_text):
        m = re.search(r'\.run-card\.live\s*\{([^}]*)\}', css_text)
        assert m, ".run-card.live rule not found"
        assert re.search(r'border-color\s*:\s*#1a3a5c', m.group(1))

    def test_run_card_live_box_shadow(self, css_text):
        m = re.search(r'\.run-card\.live\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'box-shadow', m.group(1))


class TestRunCardFailed:
    def test_run_card_failed_exists(self, css_text):
        assert '.run-card.run-failed' in css_text

    def test_run_card_failed_border_left(self, css_text):
        m = re.search(r'\.run-card\.run-failed\s*\{([^}]*)\}', css_text)
        assert m, ".run-card.run-failed rule not found"
        assert re.search(r'border-left\s*:', m.group(1))


class TestLogPanel:
    def test_log_panel_exists(self, css_text):
        assert '.log-panel' in css_text

    def test_log_panel_border_top(self, css_text):
        m = re.search(r'(?<![.\w])\.log-panel\s*\{([^}]*)\}', css_text)
        assert m, ".log-panel rule not found"
        assert re.search(r'border-top\s*:', m.group(1))

    def test_log_panel_hidden_exists(self, css_text):
        assert '.log-panel.hidden' in css_text

    def test_log_panel_hidden_display_none(self, css_text):
        m = re.search(r'\.log-panel\.hidden\s*\{([^}]*)\}', css_text)
        assert m, ".log-panel.hidden rule not found"
        assert re.search(r'display\s*:\s*none', m.group(1))


class TestLogToolbar:
    def test_log_toolbar_exists(self, css_text):
        assert '.log-toolbar' in css_text

    def test_log_toolbar_flex(self, css_text):
        m = re.search(r'\.log-toolbar\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'display\s*:\s*flex', m.group(1))

    def test_log_toolbar_background(self, css_text):
        m = re.search(r'\.log-toolbar\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'background\s*:\s*#111827', m.group(1))


class TestLogLabel:
    def test_log_label_exists(self, css_text):
        assert '.log-label' in css_text

    def test_log_label_monospace(self, css_text):
        m = re.search(r'\.log-label\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'font-family\s*:\s*monospace', m.group(1))


class TestLogCopyBtn:
    def test_log_copy_btn_exists(self, css_text):
        assert '.log-copy-btn' in css_text

    def test_log_copy_btn_color(self, css_text):
        m = re.search(r'\.log-copy-btn\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'color\s*:\s*#4a9eff', m.group(1))


class TestLogOutput:
    def test_log_output_exists(self, css_text):
        assert '.log-output' in css_text

    def test_log_output_background(self, css_text):
        m = re.search(r'\.log-output\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'background\s*:\s*#0d1117', m.group(1))

    def test_log_output_max_height(self, css_text):
        m = re.search(r'\.log-output\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'max-height\s*:\s*240px', m.group(1))

    def test_log_output_prewrap(self, css_text):
        m = re.search(r'\.log-output\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'white-space\s*:\s*pre-wrap', m.group(1))


class TestLiveBadge:
    def test_live_badge_exists(self, css_text):
        assert '.live-badge' in css_text

    def test_live_badge_inline_flex(self, css_text):
        m = re.search(r'\.live-badge\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'display\s*:\s*inline-flex', m.group(1))

    def test_live_badge_color(self, css_text):
        m = re.search(r'\.live-badge\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'color\s*:\s*#2196F3', m.group(1), re.IGNORECASE)


class TestLogCursor:
    def test_log_cursor_exists(self, css_text):
        assert '.log-cursor' in css_text

    def test_log_cursor_animation(self, css_text):
        m = re.search(r'\.log-cursor\s*\{([^}]*)\}', css_text)
        assert m
        assert re.search(r'animation\s*:', m.group(1))


class TestBlinkKeyframes:
    def test_blink_keyframes_exists(self, css_text):
        assert '@keyframes blink' in css_text

    def test_blink_opacity_0(self, css_text):
        # Find everything between @keyframes blink { ... } using a broader slice
        start = css_text.find('@keyframes blink')
        assert start != -1, "@keyframes blink not found"
        block_start = css_text.find('{', start)
        assert block_start != -1
        # Walk to find the matching closing brace
        depth = 0
        end = block_start
        for i, ch in enumerate(css_text[block_start:], start=block_start):
            if ch == '{':
                depth += 1
            elif ch == '}':
                depth -= 1
                if depth == 0:
                    end = i
                    break
        block_content = css_text[block_start:end + 1]
        assert 'opacity: 0' in block_content or 'opacity:0' in block_content, \
            f"Expected 'opacity: 0' inside @keyframes blink. Got: {block_content!r}"
