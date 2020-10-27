<style>
	img {
		width: 100%;
		height: 100%;
	}

	.wrapper {
		position: relative;
	}

	.red,
	.blue {
		position: absolute;
		top: 0;
	}

	.red {
		left: 3px;
		filter: drop-shadow(-2px 0 red);
		animation-name: glitch-animation-1;
		animation-duration: 10s;
		animation-timing-function: linear;
		animation-delay: 0s;
		animation-iteration-count: infinite;
		animation-direction: reverse-alternate;
	}

	.blue {
		left: -3px;
		filter: drop-shadow(-2px 0 blue);
		animation-name: glitch-animation-2;
		animation-duration: 10s;
		animation-timing-function: linear;
		animation-delay: 0s;
		animation-iteration-count: infinite;
		animation-direction: reverse-alternate;
	}

	@keyframes glitch-animation-1 {
		0% {
			clip-path: polygon(0 0%, 100% 0%, 100% 0%, 0% 0%);
		}
		90% {
			clip-path: polygon(0 0%, 100% 0%, 100% 0%, 0% 0%);
		}
		90.5% {
			clip-path: polygon(0 39%, 100% 39%, 100% 96%, 0% 96%);
		}
		91.0% {
			clip-path: polygon(0 20%, 100% 20%, 100% 44%, 0% 44%);
		}
		91.5% {
			clip-path: polygon(0 82%, 100% 82%, 100% 55%, 0% 55%);
		}
		92.0% {
			clip-path: polygon(0 42%, 100% 42%, 100% 26%, 0% 26%);
		}
		92.5% {
			clip-path: polygon(0 84%, 100% 84%, 100% 1%, 0% 1%);
		}
		93.0% {
			clip-path: polygon(0 41%, 100% 41%, 100% 84%, 0% 84%);
		}
		93.5% {
			clip-path: polygon(0 40%, 100% 40%, 100% 68%, 0% 68%);
		}
		94.0% {
			clip-path: polygon(0 28%, 100% 28%, 100% 90%, 0% 90%);
		}
		94.5% {
			clip-path: polygon(0 56%, 100% 56%, 100% 77%, 0% 77%);
		}
		95.0% {
			clip-path: polygon(0 67%, 100% 67%, 100% 76%, 0% 76%);
		}
		95.5% {
			clip-path: polygon(0 61%, 100% 61%, 100% 78%, 0% 78%);
		}
		96.0% {
			clip-path: polygon(0 24%, 100% 24%, 100% 85%, 0% 85%);
		}
		96.5% {
			clip-path: polygon(0 35%, 100% 35%, 100% 44%, 0% 44%);
		}
		97.0% {
			clip-path: polygon(0 26%, 100% 26%, 100% 20%, 0% 20%);
		}
		97.5% {
			clip-path: polygon(0 27%, 100% 27%, 100% 13%, 0% 13%);
		}
		98.0% {
			clip-path: polygon(0 79%, 100% 79%, 100% 23%, 0% 23%);
		}
		98.5% {
			clip-path: polygon(0 11%, 100% 11%, 100% 72%, 0% 72%);
		}
		99.0% {
			clip-path: polygon(0 31%, 100% 31%, 100% 6%, 0% 6%);
		}
		99.5% {
			clip-path: polygon(0 43%, 100% 43%, 100% 64%, 0% 64%);
		}
		100.0% {
			clip-path: polygon(0 28%, 100% 28%, 100% 43%, 0% 43%);
		}
	}
	@keyframes glitch-animation-2 {
		0% {
			clip-path: polygon(0 0%, 100% 0%, 100% 0%, 0% 0%);
		}
		90% {
			clip-path: polygon(0 0%, 100% 0%, 100% 0%, 0% 0%);
		}
		90.5% {
			clip-path: polygon(0 77%, 100% 77%, 100% 70%, 0% 70%);
		}
		91.0% {
			clip-path: polygon(0 23%, 100% 23%, 100% 73%, 0% 73%);
		}
		91.5% {
			clip-path: polygon(0 91%, 100% 91%, 100% 47%, 0% 47%);
		}
		92.0% {
			clip-path: polygon(0 62%, 100% 62%, 100% 6%, 0% 6%);
		}
		92.5% {
			clip-path: polygon(0 74%, 100% 74%, 100% 39%, 0% 39%);
		}
		93.0% {
			clip-path: polygon(0 68%, 100% 68%, 100% 1%, 0% 1%);
		}
		93.5% {
			clip-path: polygon(0 39%, 100% 39%, 100% 33%, 0% 33%);
		}
		94.0% {
			clip-path: polygon(0 53%, 100% 53%, 100% 30%, 0% 30%);
		}
		94.5% {
			clip-path: polygon(0 23%, 100% 23%, 100% 54%, 0% 54%);
		}
		95.0% {
			clip-path: polygon(0 76%, 100% 76%, 100% 53%, 0% 53%);
		}
		95.5% {
			clip-path: polygon(0 24%, 100% 24%, 100% 21%, 0% 21%);
		}
		96.0% {
			clip-path: polygon(0 25%, 100% 25%, 100% 76%, 0% 76%);
		}
		96.5% {
			clip-path: polygon(0 95%, 100% 95%, 100% 30%, 0% 30%);
		}
		97.0% {
			clip-path: polygon(0 74%, 100% 74%, 100% 67%, 0% 67%);
		}
		97.5% {
			clip-path: polygon(0 93%, 100% 93%, 100% 46%, 0% 46%);
		}
		98.0% {
			clip-path: polygon(0 35%, 100% 35%, 100% 23%, 0% 23%);
		}
		98.5% {
			clip-path: polygon(0 13%, 100% 13%, 100% 57%, 0% 57%);
		}
		99.0% {
			clip-path: polygon(0 46%, 100% 46%, 100% 82%, 0% 82%);
		}
		99.5% {
			clip-path: polygon(0 15%, 100% 15%, 100% 99%, 0% 99%);
		}
		100.0% {
			clip-path: polygon(0 92%, 100% 92%, 100% 91%, 0% 91%);
		}
	}
</style>

<div class="wrapper">
	<img alt="Logo" class="red" src="logo.png" />
	<img alt="Logo" src="logo.png" />
	<img alt="Logo" class="blue" src="logo.png" />
</div>
