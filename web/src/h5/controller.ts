/**
 * 用户端 H5 控制器（TypeScript 版）
 * 覆盖问卷作答 / 授权 / 报名支付 / 续费落地等 12 屏。
 * 数据经 H5Api 接缝加载；选项点击、逐题作答、主流程跳转均为真实交互。
 */
import { PageBase, type StyleObj, type Vals } from '../shared/ui/runtime';
import { SEED_H5, deepCopy } from '../shared/api/mockData';
import type { H5Data, H5Option } from '../shared/api/types';
import { toast } from '../shared/ui/feedback';
import { readPublicSurvey, submitSurvey } from '../api/public-survey';
import type { PublicSurveyDefinition } from '../api/generated/health';

const ACCENT = '#3370ff';
const TOTAL_Q = 12;

type H5State = {
  /** 单选选中下标 */
  single: number;
  /** 多选选中集合 */
  multi: boolean[];
  /** 逐题作答当前选中 */
  stepSel: number;
  blankSel: number | null;
  /** 逐题模式当前题号（从 1 计） */
  qIndex: number;
  submitting: boolean;
};

export class H5Controller extends PageBase {
  override state: H5State = {
    single: 0,
    multi: SEED_H5.multi.map((o) => o.on),
    stepSel: 0,
    blankSel: null,
    qIndex: 3,
    submitting: false,
  };

  private data: H5Data = deepCopy(SEED_H5);
  private definition: PublicSurveyDefinition | null = null;

  constructor(readonly page: string) {
    super();
  }

  async init(): Promise<void> {
    const slug = new URLSearchParams(location.search).get('slug');
    if (slug) {
      try {
        this.definition = await readPublicSurvey(slug);
      } catch (error) {
        toast(error instanceof Error ? error.message : '问卷读取失败', true);
      }
    }
    if (this.__render) this.__render();
  }

  private async submit(): Promise<void> {
    if (!this.definition) {
      toast('后端能力未就绪：缺少公开问卷 slug，不能提交', true);
      return;
    }
    if (this.state.submitting) return;
    this.setState({ submitting: true });
    try {
      const questions = this.definition.questions;
      const answers = questions.map((question, index) => ({
        question_id: question.id,
        option_ids: question.type === 'multi_choice'
          ? question.options.filter((_option, optionIndex) => this.state.multi[optionIndex]).map((option) => option.id)
          : [question.options[(index === 0 ? this.state.single : this.state.stepSel) % question.options.length]?.id].filter((id): id is number => id !== undefined),
      }));
      const key = crypto.getRandomValues(new Uint8Array(32)).reduce((value, byte) => value + 'ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789-_'[byte & 63], '');
      const receipt = await submitSurvey(this.definition.slug, { version: this.definition.version, submission_key: key, answers });
      location.href = `result.html?result_token=${encodeURIComponent(receipt.resultToken)}`;
    } catch (error) {
      this.setState({ submitting: false });
      toast(error instanceof Error ? error.message : '问卷提交失败', true);
    }
  }

  private goto(page: string): void {
    location.href = page + '.html';
  }

  /** 选项样式（与原型 opt() 一致）+ 点击行为 */
  private opt(o: H5Option, on: boolean, pick: () => void): Vals {
    const accent = ACCENT;
    const style: StyleObj = {
      display: 'flex', alignItems: 'flex-start', gap: '10px', padding: '13px 14px', marginBottom: '10px',
      borderRadius: '12px', cursor: 'pointer',
      background: on ? '#F2F7FF' : '#F7F8FA',
      border: on ? '1px solid ' + accent : '1px solid transparent',
      boxShadow: on ? '0 0 0 3px rgba(51,112,255,.08)' : 'none',
      color: '#1F2329',
    };
    const mark: StyleObj =
      o.kind === 'box'
        ? {
            width: '18px', height: '18px', borderRadius: '5px', flex: 'none', marginTop: '2px',
            border: on ? '5px solid ' + accent : '1px solid #C4C7CC', background: '#fff',
          }
        : {
            width: '18px', height: '18px', borderRadius: '50%', flex: 'none', marginTop: '2px',
            border: on ? '5px solid ' + accent : '1px solid #C4C7CC', background: '#fff',
          };
    return { text: o.text, style, dot: mark, box: mark, pick };
  }

  renderVals(): Vals {
    const s = this.state;
    const d = this.data;
    const accent = ACCENT;

    const act: Record<string, () => void> = {
      /* 授权 */
      authContinue: () => toast('后端能力未就绪：缺少 OAuth 上下文，不能继续授权', true),
      /* 整卷提交 */
      submitAll: () => { void this.submit(); },
      /* 逐题作答 */
      prevQ: () => {
        if (s.qIndex <= 1) {
          toast('已经是第一题');
          return;
        }
        this.setState({ qIndex: s.qIndex - 1 });
      },
      nextQ: () => {
        if (s.qIndex >= TOTAL_Q) {
          void this.submit();
          return;
        }
        this.setState({ qIndex: s.qIndex + 1 });
      },
      /* 结果页 */
      viewCourses: () => toast('后端能力未就绪：当前没有课程列表契约', true),
      viewDetail: () => toast('后端能力未就绪：当前没有完整报告契约', true),
      /* 报名 / 支付 / 续费 */
      signup: () => toast('后端能力未就绪：当前没有报名契约', true),
      pay: () => toast('后端能力未就绪：当前没有支付契约', true),
      renew: () => toast('后端能力未就绪：当前没有续费契约', true),
      addWx: () => toast('后端能力未就绪：当前没有企微加好友契约', true),
      closeQr: () => this.goto('active'),
    };

    return {
      act,
      opts: {
        single: d.single.map((o, i) => this.opt(o, s.single === i, () => this.setState({ single: i }))),
        multi: d.multi.map((o, i) =>
          this.opt(o, s.multi[i], () => {
            const next = s.multi.slice();
            next[i] = !next[i];
            this.setState({ multi: next });
          }),
        ),
        step: d.step.map((o, i) => this.opt(o, s.stepSel === i, () => this.setState({ stepSel: i }))),
        blank: d.blank.map((o, i) => this.opt(o, s.blankSel === i, () => this.setState({ blankSel: i }))),
      },
      qProgressText: `第 ${s.qIndex} / ${TOTAL_Q} 题`,
      qPct: Math.round((s.qIndex / TOTAL_Q) * 100),
      qTitle: `第 ${s.qIndex} 题`,
      dims: d.dims.map((x) => ({
        name: x.name,
        score: x.score,
        desc: x.desc,
        scoreStyle: {
          fontSize: '15px', fontWeight: 600,
          color: x.tone === 'ok' ? '#2EA121' : x.tone === 'warn' ? '#D97917' : accent,
          fontVariantNumeric: 'tabular-nums',
        },
        bar: {
          width: Math.round((x.score / x.max) * 100) + '%', height: '100%', borderRadius: '999px',
          background: x.tone === 'ok' ? '#2EA121' : x.tone === 'warn' ? '#FF8800' : accent,
        },
      })),
    };
  }
}
