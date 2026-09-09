package report

const htmlTemplate = `<!DOCTYPE html>
<html>
<head>
    <meta charset="UTF-8">
    <title>Option Replay Report</title>
    <script src="https://cdn.jsdelivr.net/npm/echarts@5.5.1/dist/echarts.min.js"></script>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        html, body { height: 100%; overflow: hidden; }
        body { background: #1a1a2e; color: #eee; font-family: 'Segoe UI', Arial, sans-serif; }
        .header { text-align: center; padding: 6px 0 4px; height: 32px; }
        .header h1 { font-size: 15px; color: #aaa; display: inline; margin-right: 12px; }
        .header label { font-size: 12px; color: #888; }
        .header select { margin-left: 5px; margin-right: 16px; background: #16213e; color: #eee; border: 1px solid #444; border-radius: 4px; padding: 1px 8px; font-size: 12px; cursor: pointer; }
        .header select:focus { outline: none; border-color: #5470c6; }
        .container { display: flex; flex-direction: column; height: calc(100% - 32px); transition: all 0.3s ease; }
        .row-top { display: flex; flex: 2; min-height: 0; transition: all 0.3s ease; }
        .row-bottom { display: flex; flex: 1; min-height: 0; transition: all 0.3s ease; }
        .container.enlarged .row-top { flex: 1; }
        .container.enlarged .row-bottom { flex: 2; }
        .chart-container { flex: 1; min-width: 0; min-height: 0; }
    </style>
</head>
<body>
<div class="header">
    <h1>Option Replay Report</h1>
    <label>Column: <select id="summarySelect"></select></label>
    <label>Single Day: <select id="tradeSelect"></select></label>
</div>
<div class="container" id="container">
    <div class="row-top">
        <div id="chart1" class="chart-container"></div>
        <div id="chart2" class="chart-container"></div>
    </div>
    <div class="row-bottom">
        <div id="chart3" class="chart-container"></div>
    </div>
</div>
<script>
var DATA = null;

const COLORS = [
    '#5470c6','#91cc75','#fac858','#ee6666','#73c0de','#3ba272','#fc8452','#9a60b4','#ea7ccc',
    '#48b8d0','#e06356','#7ecfa0','#c23531','#2f4554','#61a0a8','#d48265','#749f83','#ca8622',
    '#bda29a','#6e7074','#546570','#c4ccd3','#f05b72','#ef5b9c','#facc14','#e4925a','#7266d0',
    '#9573d0','#4aa3b0','#c14089','#59678c'
];

function identity(v) { return v; }

function toBase100(v) {
    if (!v || v.length === 0) return [];
    var base = v[0];
    if (base === 0) return v.slice();
    return v.map(function(x) { return x / base * 100; });
}

function computeAverage(seriesData) {
    var n = seriesData.length;
    if (n === 0) return [];
    var len = seriesData[0].length;
    var avg = new Array(len).fill(0);
    seriesData.forEach(function(data) {
        for (var i = 0; i < len; i++) {
            avg[i] += data[i];
        }
    });
    for (var i = 0; i < len; i++) {
        avg[i] = avg[i] / n;
    }
    return avg;
}

function makeTrendChart(id, title, transformFn) {
    const dom = document.getElementById(id);
    const chart = echarts.init(dom, 'dark');

    // -1 means no single-day trade highlighted.
    var curHighlight = -1;

    function buildSeries(field) {
        const series = [];
        const transformed = [];
        DATA.trades.forEach(function(trade, idx) {
            var raw = trade.summary[field] || [];
            var data = transformFn(raw);
            transformed.push(data);
            var isHigh = idx === curHighlight;
            series.push({
                name: 'Trade ' + trade.id + ' (PnL: ' + trade.pnl.toFixed(2) + ')',
                type: 'line',
                data: data,
                smooth: false,
                symbol: 'none',
                lineStyle: isHigh ? {
                    width: 4,
                    color: '#ffd54f',
                    shadowBlur: 12,
                    shadowColor: 'rgba(255,213,79,0.7)'
                } : { width: 1.5 },
                emphasis: { lineStyle: { width: 3 } },
                z: isHigh ? 9 : 4
            });
        });
        series.push({
            name: 'Average',
            type: 'line',
            data: computeAverage(transformed),
            smooth: false,
            symbol: 'none',
            lineStyle: {
                width: 3.5,
                color: '#ffffff',
                shadowBlur: 10,
                shadowColor: 'rgba(255,255,255,0.6)',
                shadowOffsetY: 1
            },
            z: 10
        });
        return series;
    }

    var defaultField = DATA.summaryTitles[DATA.summaryTitles.length - 1];

    var option = {
        backgroundColor: '#1a1a2e',
        title: { text: title, left: 'center', textStyle: { fontSize: 13, color: '#ccc' } },
        tooltip: {
            trigger: 'axis',
            axisPointer: { type: 'cross', lineStyle: { color: '#555' } },
            backgroundColor: 'rgba(20,20,40,0.9)',
            textStyle: { fontSize: 11 },
            formatter: function(params) {
                if (!params || params.length === 0) return '';
                var html = '<b>' + params[0].axisValue + '</b><br/>';
                params.forEach(function(p) {
                    html += '<span style="display:inline-block;margin-right:4px;border-radius:10px;width:10px;height:10px;background-color:' + p.color + ';"></span>';
                    html += p.seriesName + ': <b>' + (typeof p.value === 'number' ? p.value.toFixed(2) : p.value) + '</b><br/>';
                });
                return html;
            }
        },
        legend: {
            type: 'scroll',
            bottom: 0,
            textStyle: { fontSize: 10 },
            pageTextStyle: { color: '#aaa' },
            pageIconColor: '#aaa',
            pageIconInactiveColor: '#555'
        },
        grid: { left: 50, right: 20, top: 40, bottom: 60 },
        xAxis: {
            type: 'category',
            data: DATA.timestamps,
            axisLabel: { fontSize: 10 },
            axisPointer: { label: { show: true } }
        },
        yAxis: { type: 'value', scale: true, splitLine: { lineStyle: { color: '#333' } } },
        dataZoom: [
            { type: 'inside', xAxisIndex: 0 },
            { type: 'slider', xAxisIndex: 0, height: 16, bottom: 30 }
        ],
        series: buildSeries(defaultField),
        color: COLORS
    };

    chart.setOption(option);
    window.addEventListener('resize', function() { chart.resize(); });

    var curField = defaultField;
    return {
        chart: chart,
        buildSeries: buildSeries,
        setField: function(field) {
            curField = field;
            chart.setOption({ series: buildSeries(field) });
        },
        setHighlightedTrade: function(idx) {
            curHighlight = idx;
            chart.setOption({ series: buildSeries(curField) });
        }
    };
}

function makeSingleDayChart(id) {
    const dom = document.getElementById(id);
    const chart = echarts.init(dom, 'dark');

    function getSummaryTitles() {
        return DATA.summaryTitles;
    }

    function buildOption(tradeIdx, highlightField) {
        var trade = DATA.trades[tradeIdx];
        if (!trade) return null;
        var highlight = highlightField;
        var titles = getSummaryTitles();

        var vol = trade.volume || [];

        // Authored series: candlestick + prominent highlighted line.
        var authored = [];
        authored.push({
            name: 'Underlying',
            type: 'candlestick',
            data: trade.candles.map(function(c) { return [c.open, c.close, c.low, c.high]; }),
            itemStyle: {
                color: '#26a69a',
                color0: '#ef5350',
                borderColor: '#26a69a',
                borderColor0: '#ef5350'
            }
        });
        authored.push({
            name: highlight,
            type: 'line',
            yAxisIndex: 1,
            data: trade.summary[highlight] || [],
            smooth: false,
            symbol: 'none',
            lineStyle: { width: 3, color: '#ffd54f', shadowBlur: 12, shadowColor: 'rgba(255,213,79,0.7)' },
            z: 8
        });

        // Volume bars - muted, overlaid in same pane as price.
        authored.push({
            name: 'Volume',
            type: 'bar',
            xAxisIndex: 0,
            yAxisIndex: 2,
            data: vol,
            itemStyle: { color: 'rgba(120,140,180,0.18)', borderWidth: 0 },
            barMaxWidth: 6,
            z: 1
        });

        // Other summary columns (non-highlighted) - muted lines.
        titles.forEach(function(t) {
            if (t === highlight) return;
            authored.push({
                name: t,
                type: 'line',
                yAxisIndex: 1,
                data: trade.summary[t] || [],
                smooth: false,
                symbol: 'none',
                lineStyle: { width: 1.2, opacity: 0.4, color: '#8a8a9a' },
                z: 3
            });
        });

        return {
            backgroundColor: '#1a1a2e',
            title: {
                text: 'Single Day — Trade ' + trade.id + ' (Underlying + Summary)',
                left: 'center',
                textStyle: { fontSize: 13, color: '#ccc' }
            },
            tooltip: {
                trigger: 'axis',
                axisPointer: { type: 'cross', lineStyle: { color: '#555' } },
                backgroundColor: 'rgba(20,20,40,0.9)',
                textStyle: { fontSize: 11 },
                formatter: function(params) { return formatCross(params); }
            },
            axisPointer: { link: [{ xAxisIndex: 'all' }], label: { backgroundColor: '#555' } },
            legend: {
                type: 'scroll',
                bottom: 0,
                textStyle: { fontSize: 9 },
                pageTextStyle: { color: '#aaa' },
                pageIconColor: '#aaa',
                pageIconInactiveColor: '#555'
            },
            grid: [
                { left: 60, right: 60, top: 35, height: '78%' }
            ],
            xAxis: [
                { type: 'category', data: trade.timestamps, gridIndex: 0, axisLabel: { fontSize: 9 }, axisTick: { alignWithLabel: true } }
            ],
            yAxis: [
                {
                    type: 'value', gridIndex: 0, scale: true,
                    axisLabel: { fontSize: 9 }, splitLine: { lineStyle: { color: '#333' } }
                },
                {
                    type: 'value', gridIndex: 0, scale: true, show: false,
                    name: 'Summary'
                },
                {
                    type: 'value', gridIndex: 0, scale: true, show: false,
                    splitNumber: 2
                }
            ],
            dataZoom: [
                { type: 'inside', xAxisIndex: 0, start: 0, end: 100 },
                { type: 'slider', xAxisIndex: 0, height: 14, bottom: 22 }
            ],
            series: authored
        };
    }

    var curTradeIdx = 0;
    var curField = DATA.summaryTitles[DATA.summaryTitles.length - 1];
    chart.setOption(buildOption(curTradeIdx, curField));

    window.addEventListener('resize', function() { chart.resize(); });

    return {
        chart: chart,
        setTrade: function(idx) {
            curTradeIdx = idx;
            chart.setOption(buildOption(idx, curField), true);
        },
        setField: function(field) {
            curField = field;
            chart.setOption(buildOption(curTradeIdx, field), true);
        },
        resize: function() { chart.resize(); }
    };
}

function formatCross(params) {
    if (!params || params.length === 0) return '';
    var html = '<b>' + params[0].axisValue + '</b><br/>';
    params.forEach(function(p) {
        var val = p.value;
        if (p.seriesType === 'candlestick' && Array.isArray(val)) {
            html += p.seriesName + ': O=' + val[1].toFixed(2) +
                    ' C=' + val[2].toFixed(2) +
                    ' L=' + val[3].toFixed(2) +
                    ' H=' + val[4].toFixed(2) + '<br/>';
            return;
        }
        html += '<span style="display:inline-block;margin-right:4px;border-radius:10px;width:10px;height:10px;background-color:' + p.color + ';"></span>';
        html += p.seriesName + ': <b>' + (typeof val === 'number' ? val.toFixed(2) : val) + '</b><br/>';
    });
    return html;
}

// --- initApp builds all charts + dropdowns once DATA is available.
function initApp(data) {
    DATA = data;

    // --- Top two panes ---
    var chart1 = makeTrendChart('chart1', 'Summary — All Trades', identity);
    var chart2 = makeTrendChart('chart2', 'Base100 — All Trades', toBase100);
    var chart3 = makeSingleDayChart('chart3');

    // --- Column dropdown ---
    var sel = document.getElementById('summarySelect');
    DATA.summaryTitles.forEach(function(t) {
        var opt = document.createElement('option');
        opt.value = t;
        opt.textContent = t;
        sel.appendChild(opt);
    });
    sel.value = DATA.summaryTitles[DATA.summaryTitles.length - 1];
    sel.addEventListener('change', function() {
        var field = this.value;
        chart1.setField(field);
        chart2.setField(field);
        chart3.setField(field);
    });

    // --- Trade dropdown (single day) ---
    var tsel = document.getElementById('tradeSelect');
    DATA.trades.forEach(function(t, idx) {
        var opt = document.createElement('option');
        opt.value = idx;
        opt.textContent = 'Trade ' + t.id + ' (PnL: ' + t.pnl.toFixed(2) + ')';
        tsel.appendChild(opt);
    });
    tsel.value = '0';
    tsel.addEventListener('change', function() {
        var idx = parseInt(this.value, 10);
        chart3.setTrade(idx);
        chart1.setHighlightedTrade(idx);
        chart2.setHighlightedTrade(idx);
    });

    // --- Double-click enlarge toggle on chart3 ---
    var container = document.getElementById('container');
    var enlarged = false;
    chart3.chart.getZr().on('dblclick', function() {
        enlarged = !enlarged;
        container.classList.toggle('enlarged', enlarged);
        setTimeout(function() {
            chart1.chart.resize();
            chart2.chart.resize();
            chart3.chart.resize();
        }, 320);
    });
}

initApp(%s);
</script>
</body>
</html>`
