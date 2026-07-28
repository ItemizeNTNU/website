<script>
	export let rotate = 0;
</script>

<span style="transform: rotate({rotate}deg);">
	<slot />
</span>

<style>
	span {
		display: inline-block;
		height: 0.9em;
		width: 0.9em;
		vertical-align: baseline;
		vertical-align: middle;
		transition: transform 400ms;
	}
</style>
